# 钉钉渠道图片与语音支持改造方案

## 1. 目标

在钉钉（DingTalk）通道中支持用户发送的**图片**、**语音**和**文件**（如 PDF、Excel），使其在需要时被 Agent 使用，同时控制下载与 token 消耗。

- **图片**：下载后写入 MediaStore，将 `mediaRefs` 传给 `HandleMessage`；下游由 `resolveMediaRefs` 转成 base64 随请求发给支持视觉的模型（如 grok-4）。
- **语音**：下载后写入 MediaStore，同样传 `mediaRefs`；Agent 侧通过现有 **Transcriber** 将语音转写为文字并拼入 `content`，再走文本模型（性价比高，无需多模态语音模型）。
- **文件（PDF、Excel 等）**：**不直接下载进当轮消息、不丢给大模型**；采用「先记录或落盘到用户空间缓存，需要时通过工具读取」的策略（见第 9 节）。

## 2. 现状

| 项目 | 说明 |
|------|------|
| 入口 | `pkg/channels/dingtalk/dingtalk.go`：`onChatBotMessageReceived` |
| 当前行为 | 仅从 `data.Text.Content` 或 `data.Content["content"]` 取**文本**，`HandleMessage(..., content, nil, ...)`，**未处理图片/语音** |
| 下游 | `BaseChannel.HandleMessage` 已支持 `media []string`；Agent `loop.go` 已有 `transcribeAudioInMessage`、`resolveMediaRefs`，无需改其他渠道 |

改造范围仅限钉钉通道实现，其它渠道不受影响。

## 3. 钉钉消息与下载

### 3.1 消息类型与回调结构

- 钉钉 Stream 模式机器人可接收：文本、图片、语音、文件、视频等。
- 单聊中可直接接收语音/文件；群聊中仅接收 @ 机器人的消息。
- 回调数据来自 SDK：`*chatbot.BotCallbackDataModel`。除 `Text.Content` 外，需从原始 payload 或 `Content` 中解析：
  - **消息类型**：如 `msgtype` / `messageType`（如 `text`、`picture`、`voice`、`file` 等）。
  - **图片**：通常带 `downloadCode` 或 `pictureDownloadCode`，用于换取下载地址。
  - **语音**：带 `downloadCode`（及可能的 `mediaId`），用于下载语音文件。

若当前 Go SDK 的 `BotCallbackDataModel` 未暴露上述字段，可：
- 在注册回调前从 SDK 的原始 JSON 中解析 `msgtype`、`content`、`downloadCode` 等；或
- 向 SDK 提交 issue/PR 扩展结构体后使用。

### 3.2 下载流程（钉钉开放平台）

1. **用 downloadCode 换下载 URL**  
   调用钉钉服务端接口（参见 [下载机器人接收消息的文件内容](https://open.dingtalk.com/document/development/download-the-file-content-of-the-robot-receiving-message)），传入 `downloadCode`，获取 `downloadUrl`（及必要时的请求头，如 Cookie）。

2. **HTTP 下载文件**  
   使用 `downloadUrl` 发起 GET（或文档要求的方式）下载二进制流，写入本地临时文件。

3. **存储与引用**  
   将本地文件路径交给 `MediaStore.Store(localPath, meta, scope)`，得到 `media://xxx`，加入本条消息的 `mediaRefs`。

图片消息与语音消息均适用上述流程（可能接口路径或参数略有不同，以官方文档为准）。

## 4. 改造步骤

### 4.1 解析消息类型与 content

- 在 `onChatBotMessageReceived` 中，除现有 `data.Text.Content` / `data.Content` 外：
  - 从回调中解析 **msgtype**（或 messageType）：`text`、`picture`、`voice`、`file` 等。
  - 从 **content**（或对应类型的 body）中解析：
    - 文本：现有逻辑保留，作为 `content` 字符串。
    - 图片：取 `downloadCode` 或 `pictureDownloadCode`。
    - 语音：取 `downloadCode`（及可选 `mediaId`）。

若当前仅支持“单条消息一种类型”，可先实现：**纯文本 / 单图 / 单语音**；后续再扩展多图、图+文等。

### 4.2 钉钉侧：下载并写入 MediaStore

- **新增**：钉钉包内实现“按 downloadCode 下载并落盘”的函数（可参考 `pkg/channels/feishu/feishu_64.go` 的 `downloadResource` 模式）：
  1. 使用钉钉 API 用 `downloadCode` 换取 `downloadUrl`（需使用应用凭证，如 access_token，见钉钉文档）。
  2. HTTP GET `downloadUrl`，将 body 写入 `$TMP/PinchBot_media/` 下唯一文件名（如 `dingtalk-<messageID>-<downloadCode 前几位>-<ext>`）。
  3. 根据 msgtype 设置扩展名与 ContentType：
     - 图片：`.jpg` 或根据接口返回；`ContentType: "image/jpeg"` 等。
     - 语音：钉钉常见为 amr；`.amr`，`ContentType: "audio/amr"`。
  4. 调用 `c.GetMediaStore().Store(localPath, media.MediaMeta{Filename, ContentType, Source: "dingtalk"}, scope)`，得到 `ref`，追加到本条消息的 `mediaRefs`。

- **scope**：使用 `channels.BuildMediaScope("dingtalk", chatID, messageID)`，与现有规范一致；若 messageID 为空可临时用 `chatID+时间戳` 等保证唯一。

- 仅当 `c.GetMediaStore() != nil` 时执行下载与 Store；否则只保留文本（与现有行为兼容）。

- **文件类（file，如 PDF、Excel）**：不在此处下载进 MediaStore、不传 mediaRefs；改为写入**文件记录**和/或**用户空间钉钉缓存目录**，当轮只给模型一句说明（见第 9 节）。

### 4.3 构造 content 与 mediaRefs，调用 HandleMessage

- **文本**：  
  - 纯文本消息：`content = data.Text.Content`（或已有解析逻辑）。  
  - 纯图片/纯语音：可设 `content = ""` 或占位如 `[图片]` / `[语音]`，由下游用 media 补充理解（见下）。

- **图片**：  
  - 下载并 Store 后得到 `mediaRefs`。  
  - 可选：在 content 中追加简短说明，例如 `[图片]`，便于纯文本场景也有提示；下游 `resolveMediaRefs` 会把 ref 转成 base64 发给模型。

- **语音**：  
  - 下载并 Store 后得到 `mediaRefs`。  
  - 不在钉钉层做转写；content 可留空或 `[语音]`。  
  - Agent 层 `transcribeAudioInMessage` 会解析 `msg.Media` 中的音频 ref，调用 Transcriber 转写，并把结果拼进 `msg.Content`（如 `[voice: 转写文本]`），因此**无需在钉钉侧集成 ASR**。

- **文件（PDF、Excel 等）**：  
  - 不传 `mediaRefs`；当轮 `content` 仅追加说明，如：`[用户上传了文件：report.pdf，可通过 read_uploaded_file 工具按需读取]`。实际内容在用户/任务要求读取时，由工具从缓存或记录中取（见第 9 节）。

- 最终调用：  
  `c.HandleMessage(ctx, peer, messageID, senderID, chatID, content, mediaRefs, metadata, sender)`  
  其中 `messageID` 若有则传入，便于去重与 scope 管理。

### 4.4 群聊与触发逻辑

- 群聊中若仅 @ 消息才会推送，现有 `ShouldRespondInGroup` 等逻辑保持不变。
- 对“仅带图片/语音、无文本”的消息：仍应视为一条需要处理的消息（content 可为占位或空），照常走 HandleMessage；Agent 侧根据 media 和转写结果生成回复。

### 4.5 错误与降级

- 下载失败或 Store 失败：打日志，不阻塞整条消息；可仅用文本 content（若有）继续处理，mediaRefs 中只包含成功的 ref。
- 未配置 MediaStore（GetMediaStore() == nil）：不下载，仅处理文本，与当前行为一致。

## 5. 数据流小结

```
用户发图/发语音
  → 钉钉 Stream 回调 onChatBotMessageReceived
  → 解析 msgtype + content（downloadCode 等）
  → 钉钉 API 用 downloadCode 换 downloadUrl，下载到本地
  → MediaStore.Store → mediaRefs
  → HandleMessage(ctx, peer, messageID, senderID, chatID, content, mediaRefs, metadata, sender)
  → 总线 → Agent
  → 若有音频：transcribeAudioInMessage 转写并写入 msg.Content
  → resolveMediaRefs 将 mediaRefs 转为 base64（图片等）随请求发给 LLM
  → 回复经钉钉 Send 回用户
```

## 6. 配置与依赖

- **配置**：无需新增配置项；MediaStore 与 Transcriber 已由 gateway 注入（钉钉通道通过 Manager 注入 MediaStore）。
- **Transcriber**：若已配置（如 Groq），语音将自动转写；未配置时语音 media 仍会传给模型，但当前 grok-4/qwen3-14b 不支持音频输入，效果依赖后续是否接入音频模型或仅依赖转写。
- **钉钉权限**：确认应用具备“接收消息”及“下载机器人接收消息的文件内容”等所需权限。

## 7. 测试建议

- 单聊：发送纯文本、单图、单语音、图+文，验证 content + mediaRefs 正确，且 Agent 能看图、能基于转写回复语音。
- 群聊：@ 机器人发送图片/语音，验证触发与回复正常。
- 下载失败：mock 或断网，验证只记录错误、不 panic，文本照常处理。
- MediaStore 未注入：验证不下载、仅文本。

## 8. 参考实现

- **媒体下载与 Store**：`pkg/channels/feishu/feishu_64.go` 中 `downloadInboundMedia`、`downloadResource`。
- **HandleMessage 传 media**：`pkg/channels/feishu/feishu_64.go`、`pkg/channels/telegram/telegram.go`、`pkg/channels/slack/slack.go` 等。
- **语音转写**：`pkg/agent/loop.go` 中 `transcribeAudioInMessage`；`pkg/voice/transcriber.go`。
- **媒体解析与发给 LLM**：`pkg/agent/loop_media.go` 的 `resolveMediaRefs`；`pkg/providers/openai_compat/provider.go` 中对 `image_url` 的处理。

---

## 9. 文件（PDF、Excel）懒加载与按需读取

### 9.1 原则

- **收到文件消息时**：不把文件内容塞进当轮消息、不直接丢给大模型，避免无谓的下载与 token 消耗。
- **只做两件事二选一或组合**：  
  - **存记录**：在内存或轻量存储中记一条「本会话/本聊天下的上传文件」（downloadCode、chatID、messageID、filename、类型、可选过期时间）。  
  - **落盘到用户空间钉钉缓存**：将文件下载到固定目录（如 `dingtalk_cache/<chatID>/<fileId>_<safe_filename>`），不经过 MediaStore、不进入当轮 LLM 请求。
- **当用户/任务明确要求读文件时**：由 Agent 调用**工具**（如 `read_uploaded_file`）按需读取；工具再根据记录或缓存路径取文件（若仅存记录则此时再下载并可选写入缓存），对 PDF/Excel 做文本抽取后返回给模型。

### 9.2 缓存目录与记录

- **缓存根目录**：可配置，例如 `config.agents.defaults.workspace` 下的 `dingtalk_cache`，或单独配置项 `dingtalk_file_cache_dir`。按 **chatID** 分子目录，便于按会话隔离与清理：`<cache_root>/<chatID>/<file_id>_<原始文件名>`。
- **文件记录**（若采用「先记录、懒下载」）：结构建议包含 `channel=dingtalk`、`chatID`、`messageID`（若有）、`downloadCode`、`filename`、`contentType`/扩展名、`cachedPath`（若已下载则填，避免重复下）。存储可用：当前进程内存 map（按 chatID 索引）、或小文件/DB（如 JSON 文件、SQLite），并设 TTL 或条数上限，避免堆积。

### 9.3 收到文件消息时的行为

1. 解析出 `msgtype=file`（或等价）及 `downloadCode`、`filename` 等。
2. **选项 A（仅记录）**：写入一条文件记录，不发起下载。当轮发给 `HandleMessage` 的 `content` 为：原有文本（若有）+ `[用户上传了文件：xxx.pdf，可通过 read_uploaded_file 工具按需读取]`，`mediaRefs` 为空。
3. **选项 B（立即落盘缓存）**：调用钉钉 API 用 `downloadCode` 换 `downloadUrl` 并下载到 `dingtalk_cache/<chatID>/<id>_<filename>`，并写入记录（含 `cachedPath`）。当轮同样不传 mediaRefs，只传上述 content 说明。
4. 推荐：**默认仅记录，懒下载**；若希望减少首次读取延迟，可对「小文件」或「已知可缓存类型」在收到时即下载到用户空间缓存。

### 9.4 工具设计：按需读取

- **工具名**：如 `read_uploaded_file`（或 `read_cached_file`）。
- **参数**：`chat_id`（必填），`file_id` 或 `file_name`（可选，不传则列出本会话最近若干条文件记录供用户选择）。
- **行为**：  
  - 根据 `chat_id`（及可选 `file_id`/`file_name`）查文件记录。  
  - 若记录中有 `cachedPath` 且文件存在，直接读该路径。  
  - 若无缓存：用记录的 `downloadCode` 调钉钉 API 下载，落盘到 `dingtalk_cache/<chatID>/...`，更新记录的 `cachedPath`，再读。  
  - 根据扩展名/MIME：PDF、Excel（xlsx/xls）做**文本抽取**（如用现有或新增的 extract 库），返回纯文本；其它类型可返回「仅支持 PDF/Excel 文本提取」或返回本地路径说明。  
  - 将抽取结果（或错误信息）作为工具结果返回给 Agent，由模型基于该文本回答用户（如总结、问答）。  

这样：**直接下载和直接丢大模型的消耗都避免**；只有用户明确要读文件时才会下载（或读缓存）并消耗一次工具调用的 token。

### 9.5 与图片/语音的区分

| 类型   | 收到时行为           | 当轮是否进 LLM       | 按需方式       |
|--------|----------------------|----------------------|----------------|
| 图片   | 下载 → MediaStore    | 是（base64 进模型）  | 无需工具       |
| 语音   | 下载 → MediaStore    | 是（转写后进模型）   | 无需工具       |
| 文件   | 记录 或 缓存到目录   | 否，仅说明性文案     | 工具读取后返回 |

---

**结论**：仅需在 `pkg/channels/dingtalk/dingtalk.go` 内扩展 `onChatBotMessageReceived`（解析 msgtype、downloadCode）：图片/语音仍为「下载 → MediaStore → mediaRefs → HandleMessage」，由现有多模态/转写 pipeline 处理；**文件（PDF、Excel）** 采用「只记录或落盘用户空间钉钉缓存、不塞进当轮消息，需要时通过 `read_uploaded_file` 工具读取」，避免直接下载和直接丢大模型带来的消耗。
