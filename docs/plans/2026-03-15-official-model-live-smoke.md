# 2026-03-15 官方模型真实联调记录

## 目标

- 使用真实 Supabase 注册/登录链路登录平台
- 使用真实官方模型上游配置 1 个可售 official model
- 验证模型目录、访问状态、管理员手动充值、官方聊天代理与失败退款回滚

## 联调环境

- 平台服务：本地 Docker 启动 `platform-server`
- 认证：真实 Supabase 项目
- 钱包：本地内存存储（本轮仅验证真实认证 + 真实上游 + 真实平台代理链路）
- 官方模型上游：
  - 协议：OpenAI 兼容
  - 模型：`gpt-5.2`
  - Base URL：已按用户提供的真实地址接入本地运行时配置
  - API Key：仅写入本地非提交运行时配置，未入库、未提交

## 本地运行时配置结果

- `official_routes`
  - `public_model_id = official-gpt-5-2`
  - `model_config.model = openai/gpt-5.2`
- `official_models`
  - 已上架 1 个模型：`official-gpt-5-2`
- `pricing_rules`
  - 已配置 fallback reserve / charge 规则
- `agreements`
  - 已配置用户协议、隐私政策、充值说明

## 实际验证结果

### 1. 平台登录链路

结果：通过

- 使用真实邮箱密码经 `/auth/login` 成功获取平台 session
- `/me`、`/official/models`、`/official/access` 在 bearer token 下可正常访问

### 2. 官方模型目录与访问状态

结果：通过

- `/official/models` 返回 1 个启用中的 official model
- `/official/access` 正确返回：
  - `enabled = true`
  - `models_configured = 1`
  - 钱包余额与充值后状态一致

### 3. 管理员手动充值

结果：通过

- 使用管理员身份调用 `/admin/manual-recharges`
- 用户钱包余额成功增加
- 钱包流水写入 `admin_manual_recharge`

### 4. 官方聊天平台代理链路

结果：**平台链路通过，但上游业务调用失败**

- 调用 `/chat/official` 时：
  - 平台成功识别模型目录与路由
  - 平台先执行预扣费 reserve
  - 平台成功向真实上游发起 OpenAI 兼容请求
- 真实上游返回：
  - HTTP `503 Service Unavailable`
  - 错误摘要：`no_available_providers`

### 5. 失败回滚 / 钱包一致性

结果：通过

- 上游返回 503 后，平台未产生最终消费
- 预扣费已自动退款回滚
- 验证到一对对应流水：
  - `official platform chat reserve`
  - `official platform chat reserve refund`
- 用户最终余额与请求前一致

## 结论

本轮已经确认：

1. **PinchBot 官方模型平台化链路本身可用**
   - 真实注册/登录
   - official model 目录展示
   - 官方访问状态
   - 管理员手动充值
   - 官方聊天代理
   - 失败退款回滚

2. **当前未完成“成功生成内容并落 usage/真实扣费”的唯一阻塞点在外部上游**
   - 当前真实上游返回 `503 no_available_providers`
   - 这不是平台路由、认证或钱包链路错误

## 第二轮切换上游后的结果

后续将 official route 切换到另一个 `/responses` 风格上游，并新增了 `responses` provider 协议支持。

### 接入结果

- 新上游：
  - Base URL：新的 `/codex/v1` 风格入口
  - 能力特征：
    - `/chat/completions` 不可用
    - `/responses` 可用
- 代码补充：
  - 新增 `responses` provider
  - `CreateProviderFromConfig` 现支持：
    - `responses/<model>`
    - `openai-responses/<model>`

### 第二轮实际验证结果

结果：通过

- 使用官方模型 `official-gpt-5-2`
- `/chat/official` 成功返回 `pong`
- usage 成功落账：
  - `prompt_tokens = 24`
  - `completion_tokens = 5`
  - `charged_fen = 1`
  - `request_kind = proxy`
- 钱包从 500 分变为 499 分
- 管理后台 usage 查询可看到对应记录

### 第二轮结论

至此可以确认：

- **官方模型真实联调已经跑通成功路径**
  - 真实登录
  - 真实 official model
  - 真实上游调用
  - 平台代理
  - usage 记录
  - 最终扣费
  - 管理端可查询

## 剩余动作

要完成“官方模型真实联调”这一项的最终闭环，还需要上游具备至少 1 个可用 provider 配额或路由能力，然后复跑以下步骤：

1. 登录
2. 管理员手动充值
3. 调用 `/chat/official`
4. 验证成功响应
5. 验证：
   - `chat_usage_records`
   - 最终扣费流水
   - pricing version 命中
   - 管理端 usage / wallet / audit 展示
