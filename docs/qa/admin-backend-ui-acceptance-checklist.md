# Admin Backend / UI Acceptance Checklist

> 验证日期：2026-03-14  
> 分支：`integration/platform-refactor`

## 已完成的自动化验证

- [x] `Platform`: `go test ./...`
- [x] `Launcher/app-wails`: `go test ./...`
- [x] `PinchBot`: `go test ./...`
- [x] 仓库差异格式检查：`git diff --check`

## 本轮重点修复与验收项

- [x] 管理后台静态壳已具备登录页、顶部状态区、侧边导航、模块化内容区
- [x] `/admin/me` 返回当前管理员身份、角色、能力集
- [x] 高风险后台写接口继续保留服务端 RBAC 校验，不依赖前端隐藏按钮
- [x] 未知管理员角色会被拒绝，不再默认升级为 `super_admin`
- [x] 管理员账号首次绑定后，不能仅凭同邮箱切换到其他 `user_id`
- [x] 侵权举报 `evidence_urls` 仅允许 `http/https`，危险 scheme 会被后端拒绝
- [x] 管理后台前端对证据链接增加二次安全拦截，危险链接不会生成可点击 `href`
- [x] 用户详情页在缺少 `agreements.read` 时不会暴露协议接受记录
- [x] 确认弹窗支持键盘焦点约束与关闭后焦点恢复
- [x] 用户详情异步加载增加请求序号保护，避免快速切换用户时旧响应覆盖新内容
- [x] 目录 / 治理编辑器改为“预览优先 + 高级 JSON 编辑器显式展开”
- [x] 数据表格补充无障碍 `caption`

## 建议人工回归清单

- [ ] 桌面宽度（>= 1280px）完整走查：登录 → Dashboard → Users → Operators → Orders → Wallet → Audits → Refunds → Infringement → Catalog → Governance
- [ ] 平板宽度（约 1024px）验证侧边栏展开 / 收起与模块切换
- [ ] 键盘专用操作验证：登录表单、侧边导航、危险操作确认弹窗
- [ ] 使用真实运行时配置验证目录模块读取 / 保存 / 回滚路径
- [ ] 使用真实官方模型配置验证“模型目录 → 聊天 → 计费 → 退款 / 侵权”运营闭环

## 当前结论

- 后台核心 API、RBAC、主要管理界面、关键安全修复与自动化验证已完成。
- 进入对外发布前，仍建议补做一次基于真实平台配置的人工联调与发版验收。
