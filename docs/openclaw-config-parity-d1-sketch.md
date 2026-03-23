# OpenClaw 配置对齐速查（D.1 草案）

用于「迁移 OpenClaw `config`」时的**字段级**对照（非完整规范）。PinchBot 权威结构见 `PinchBot/pkg/config/config.go`；OpenClaw 参考类型见 `PinchBot/pkg/migrate/sources/openclaw/openclaw_config.go`。

| 区域 | OpenClaw（概念） | PinchBot 现状 | 备注 |
|------|------------------|---------------|------|
| `agents.defaults` / `agents.list[]` | `model`、`workspace`、`tools`（profile/allow/deny/alsoAllow）等 | 多数字段已存在；**`tools`** 合并与 deny 规则已用于 invoke + LLM + 子循环（见 D.2） | `identity` 等按需补 |
| `tools`（全局） | profile、allow、deny | 以 **`tools.*.enabled`** 等细粒度开关为主；与 OpenClaw `tools` 对象**不完全同构** | 迁移脚本需映射表 |
| `gateway` | 监听、鉴权、工具 HTTP 策略 | **`GatewayConfig`**：`host`/`port`/`auth`/`tools`/`rate_limit` | 与上游键名可能不同 |
| `session` | 会话策略 | **`SessionConfig`**（dm_scope、identity_links 等） | 部分对齐 |
| `channels.*` | 各渠道大块配置 | **`ChannelsConfig`** | 按渠道逐步对齐 |
| `hooks` / `cron` / `memory` | 各 JSON 块 | 分散在 `pkg/cron`、`graphmemory` 等 | 深度对齐属 D.1 后续迭代 |

**建议**：迁移时先冻结 **PinchBot `config.json` 黄金样例** + **OpenClaw 导出 JSON**，用 `pkg/migrate` 已有路径试跑，再补字段级测试（fixture 对比）。
