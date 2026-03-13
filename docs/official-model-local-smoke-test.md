# 官方模型本地联调 Smoke Test

本文用于收口 IMPL-7：在本地或准生产环境里，把 **桌面登录 / 官方模型 / 钱包 / 充值协议** 跑成一条可验证链路。

## 1. 准备 live 配置

优先使用仓库脚本从 example 文件生成 live 文件：

### macOS / Linux

```bash
./scripts/bootstrap-local-platform-config.sh
```

### Windows PowerShell

```powershell
.\scripts\bootstrap-local-platform-config.ps1
```

生成结果：

- `Platform/config/platform.env`
- `Platform/config/runtime-config.json`

> 这一步只是复制模板，仍需手动替换真实值，尤其不要保留 `replace-with-your-upstream-api-key`。

## 2. 必改项

### `Platform/config/platform.env`

至少确认：

- `PLATFORM_DATABASE_URL`
- `PLATFORM_SUPABASE_URL`
- `PLATFORM_SUPABASE_PUBLISHABLE_KEY`
- `PLATFORM_SUPABASE_SERVICE_ROLE_KEY`
- `PLATFORM_PAYMENT_PROVIDER`
- `PLATFORM_RUNTIME_CONFIG_PATH`

### `Platform/config/runtime-config.json`

至少确认：

- `official_routes[0].model_config.api_key`
- `official_routes[0].model_config.api_base`
- `official_models[0].enabled = true`
- `pricing_rules[0].version`
- `agreements[*].url`

## 3. 启动

### macOS / Linux

```bash
./scripts/start-local-platform.sh
```

### Windows PowerShell

```powershell
.\scripts\start-local-platform.ps1
```

## 4. 联调检查清单

- [ ] 启动桌面端后先出现注册 / 登录弹窗
- [ ] 能完成注册或登录
- [ ] 登录后能看到官方模型列表
- [ ] `official-basic` 或你替换后的官方模型可被选中
- [ ] 设置页能读取余额、订单、协议信息
- [ ] 创建充值订单时会要求先接受充值相关协议
- [ ] 钱包订单详情接口 `GET /wallet/orders/{id}` 正常返回
- [ ] 官方模型聊天请求可成功到达上游
- [ ] 成功请求会形成消费记录
- [ ] 余额不足时会被禁止继续调用

## 5. 支付 / 对账补充验证

如果本地已接入易支付或测试支付环境，再额外检查：

- [ ] 支付回调能把 pending 订单改成 paid
- [ ] `POST /admin/orders/{id}/reconcile` 可用于单笔补对账
- [ ] `POST /admin/orders/reconcile-pending` 可用于批量补对账
- [ ] 退款申请后可进入 `approved_pending_payout` 或 `refunded`
- [ ] 手工出款后可调用 `POST /admin/refund-requests/{id}/settle`

## 6. 常见失败点

- `platform.env` 还在用 example 占位值
- `runtime-config.json` 仍保留 `replace-with-your-upstream-api-key`
- 官方模型是 enabled，但没有对应 `official_routes`
- `pricing_rules.version` 没有随模型路由一起更新
- `agreements` 中 URL / 内容还是示例文本，导致前端知情确认页面不符合上线要求
