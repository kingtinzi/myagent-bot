# MyAgent Bot 平台化重构剩余事项 Todo

> 更新日期：2026-03-14
> 基线计划：`docs/plans/2026-03-12-myagent-bot-platform-refactor-final.md`

## 本轮已完成

以下内容已不再属于“剩余事项”：

- 应用账号注册 / 登录 / 登出链路打通
- 注册前协议接受校验与充值前知情材料链路
- 官方模型平台接口、余额、订单、计费、退款、侵权举报基础闭环
- 管理后台核心 API：
  - `/admin/me`
  - `/admin/dashboard`
  - `/admin/users`
  - `/admin/users/{id}/overview`
  - `/admin/users/{id}/wallet-transactions`
  - `/admin/users/{id}/orders`
  - `/admin/users/{id}/agreements`
  - `/admin/users/{id}/usage`
  - `/admin/operators`
  - `/admin/orders`
  - `/admin/wallet-adjustments`
  - `/admin/model-routes`
  - `/admin/pricing-rules`
  - `/admin/agreement-versions`
  - `/admin/audit-logs`
  - `/admin/refund-requests`
  - `/admin/infringement-reports`
  - `/admin/system-notices`
  - `/admin/risk-rules`
  - `/admin/data-retention-policies`
- 管理后台 UI 主壳、RBAC、危险操作确认、用户详情钻取、治理 / 财务 / 目录模块
- 桌面端接口形态与最终方案对齐：
  - `GetOfficialAccessState()`
  - `ListOfficialModels()`
  - `GetBackendStatus()`
- Windows / macOS 打包脚本、README 与安装器脚本基础能力

## 当前仍然剩余的事项

## P1 应做

### 1. 正式支付 Phase 2

- 接入微信 / 支付宝正式网关
- 异步回调验签与幂等
- 对账与失败补偿
- 生产级订单状态监控

目标：
- 从“易支付 / 模拟支付可联调”升级到“正式支付可运营”。

### 2. 官方模型真实联调

- 至少配置 1 个真实可售 official model
- 跑通展示、聊天、计费、扣费、退款回写
- 校验管理员改路由 / 改价格后的实时生效行为

目标：
- 从“接口闭环完成”升级到“真实业务闭环已验证”。

当前状态（2026-03-15）：

- 已完成：
  - 真实 Supabase 登录联调
  - 本地非提交运行时配置接入真实 official model 上游
  - `/official/models`、`/official/access` 验证通过
  - 管理员手动充值验证通过
  - 第一轮失败上游已验证退款回滚
  - 第二轮切换 `/responses` 风格上游后，`/chat/official` 成功返回
  - usage 落账、最终扣费、管理端 usage 查询已验证通过
- 管理员改路由 / 改价格后的实时生效已完成专项验收
- 记录：
  - 详见 `docs/qa/2026-03-15-official-model-live-smoke.md`
  - 详见 `docs/qa/2026-03-15-official-model-route-pricing-live-acceptance.md`

## P2 可后置

### 3. 管理后台继续产品化

- 非 JSON 的结构化治理编辑器
- 更细粒度的审计检索 / 导出
- 更完整的统计看板与时间维度筛选

目标：
- 从“生产可用后台”继续提升为“更强运营工具”。

### 4. 最终发版闭环

- Windows 代码签名
- macOS codesign / notarization / DMG 终验
- clean machine 安装验收
- 交付版回归清单

目标：
- 从“工程打包可用”推进到“可正式对外交付”。

当前状态（2026-03-15）：

- 已完成：
  - Windows 打包脚本、安装器模板与 README 基础能力
  - macOS 打包脚本、notarization 脚本、DMG 打包脚本
  - Windows 发布/签名 Runbook：`docs/release-windows-runbook.md`
  - Windows 签名辅助脚本：`scripts/sign-windows.ps1`
  - macOS 发布 Runbook：`docs/release-macos-runbook.md`
- 当前剩余：
  - 真实证书执行 Windows Authenticode 签名
  - 真实 Apple 开发者凭据执行 codesign / notarization / staple
  - 在干净 Windows / 干净 Mac 上完成最终安装验收

## 建议执行顺序

1. 正式支付 Phase 2
2. 官方模型真实联调
3. Windows / macOS 最终签名与发版验收
4. 管理后台持续产品化增强
