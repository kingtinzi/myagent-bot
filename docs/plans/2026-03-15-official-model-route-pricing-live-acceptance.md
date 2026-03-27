# 2026-03-15 官方模型路由 / 定价实时生效验收

## 验收目标

验证管理员后台修改以下配置后，无需重启平台服务即可对新请求立即生效：

- 官方路由 `GET/PUT /admin/model-routes`
- 定价规则 `GET/PUT /admin/pricing-rules`

## 验收环境

- 平台服务：本地 `platform-server`
- 管理入口：管理员 bearer token
- 测试用户：真实 Supabase 账号
- 官方模型：`official-gpt-5-2`

## 验收步骤与结果

### 1. 路由从“可用 Responses 上游”切到“失败 Chat Completions 上游”

操作：

- 读取 `/admin/model-routes`
- 使用返回的 revision/ETag 调 `PUT /admin/model-routes`
- 将 `official-gpt-5-2` 路由切到失败上游

结果：

- 路由更新成功
- 紧接着的新 `/chat/official` 请求立即失败
- 平台返回上游 503 映射后的错误
- 钱包余额保持不变，说明失败请求已正确退款回滚

### 2. 路由切回“可用 Responses 上游”

操作：

- 再次读取 `/admin/model-routes`
- 使用新的 revision/ETag 调 `PUT /admin/model-routes`
- 将 `official-gpt-5-2` 路由切回可用上游

结果：

- 路由更新成功
- 紧接着的新 `/chat/official` 请求立即成功
- 返回内容：`pong`
- 当次请求计费：`charged_fen = 1`

这证明：

- **管理员改路由后，新请求立即按新路由生效**
- **不需要重启平台服务**

### 3. 定价从 1 分切到 3 分

操作：

- 读取 `/admin/pricing-rules`
- 使用返回的 revision/ETag 调 `PUT /admin/pricing-rules`
- 将 `official-gpt-5-2` 的价格版本切到：
  - `official-gpt-5-2@2026-03-15-r2`
- 调高单价，使同样的测试请求按 3 分结算

结果：

- 定价更新成功
- 紧接着的新 `/chat/official` 请求立即成功
- 返回：
  - `charged_fen = 3`
  - `pricing_version = official-gpt-5-2@2026-03-15-r2`
- 管理端 usage 查询同步显示：
  - 最新 usage 记录版本为新定价版本
  - 同请求 token 用量下，扣费从 1 分变成 3 分

这证明：

- **管理员改价格后，新请求立即按新价格生效**
- **历史 usage 仍保留旧版本，新的 usage 正确命中新版本**

## 结论

本轮已经完成以下生产级验收：

- 管理员改路由实时生效
- 管理员改价格实时生效
- revision / ETag 并发控制可用于安全更新配置
- 失败路由会触发退款回滚
- 成功路由会写入 usage 与最终扣费

因此，“官方模型真实联调”主线项现在可以视为：

- **成功路径已验证**
- **失败回滚已验证**
- **后台变更实时生效已验证**
