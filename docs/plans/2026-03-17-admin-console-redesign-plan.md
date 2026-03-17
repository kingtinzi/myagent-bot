# PinchBot 管理后台重构 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将现有单文件 `admin_index.html` 重构为组件化、可维护、极简企业 SaaS 风格的新版管理后台，同时尽量复用现有 Go 后端接口与权限模型。

**Architecture:** 新建 `Platform/admin-console/` React + TypeScript + Vite 前端工程，按业务域拆分页面、组件、状态与数据访问层；构建产物由 `Platform/internal/api` 继续托管，替换当前单文件后台入口。迁移过程采用渐进式切换：先建立新版后台壳层与设计系统，再接入核心业务页，最后接入治理页、管理员页并收口旧后台。

**Tech Stack:** React、TypeScript、Vite、React Router、TanStack Query、Zustand、Zod、React Testing Library、Vitest、Go `embed`、现有 `Platform/internal/api` 服务端。

---

## 实施约束

- 不提交本地私有配置文件：`Platform/config/runtime-config.json`
- 现有 Go 接口尽量复用，不先改 API 语义，优先前端重构
- 新后台必须保留现有权限模型：模块权限、读权限、写权限、危险操作确认
- 列表页统一“筛选条 + 表格 + 详情抽屉”，配置页统一“对象列表 + 结构化编辑 + 风险说明”
- UUID 不作为主显示身份，统一优先显示用户名、用户编号、邮箱
- 所有危险操作统一二次确认体验，保留后端审计与服务端权限校验

---

### Task 1: 建立新版后台前端工程骨架

**Files:**
- Create: `Platform/admin-console/package.json`
- Create: `Platform/admin-console/tsconfig.json`
- Create: `Platform/admin-console/tsconfig.node.json`
- Create: `Platform/admin-console/vite.config.ts`
- Create: `Platform/admin-console/index.html`
- Create: `Platform/admin-console/src/main.tsx`
- Create: `Platform/admin-console/src/app/App.tsx`
- Create: `Platform/admin-console/src/app/router.tsx`
- Create: `Platform/admin-console/src/app/providers.tsx`
- Create: `Platform/admin-console/src/styles/tokens.css`
- Create: `Platform/admin-console/src/styles/global.css`
- Create: `Platform/admin-console/src/env.d.ts`
- Create: `Platform/admin-console/vitest.config.ts`
- Create: `Platform/admin-console/src/test/setup.ts`

**Step 1: 写失败测试 / 校验目标**

新增最小工程级测试文件：

- Create: `Platform/admin-console/src/app/App.test.tsx`

测试目标：
- 应用可以渲染基础壳层
- Router 可用
- Query Provider 可用

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand
```

Expected:
- 失败，提示前端工程或测试环境尚未建立

**Step 3: 写最小实现**

- 初始化 `package.json` 依赖与脚本：
  - `dev`
  - `build`
  - `test`
  - `lint`（可先留空实现或基础脚本）
- 建立 Vite + React + TS 基础入口
- 建立最小 Provider 栈：
  - `QueryClientProvider`
  - `RouterProvider`

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm install
npm test -- --runInBand
```

Expected:
- `App.test.tsx` 通过

**Step 5: Commit**

```bash
git add Platform/admin-console
git commit -m "feat: scaffold admin console frontend workspace"
```

---

### Task 2: 落地设计系统与后台壳层

**Files:**
- Create: `Platform/admin-console/src/components/layout/AdminShell.tsx`
- Create: `Platform/admin-console/src/components/layout/AdminSidebar.tsx`
- Create: `Platform/admin-console/src/components/layout/AdminTopbar.tsx`
- Create: `Platform/admin-console/src/components/layout/PageHeader.tsx`
- Create: `Platform/admin-console/src/components/feedback/GlobalToast.tsx`
- Create: `Platform/admin-console/src/components/feedback/InlineStatus.tsx`
- Create: `Platform/admin-console/src/components/display/StatusBadge.tsx`
- Create: `Platform/admin-console/src/components/display/MetricCard.tsx`
- Create: `Platform/admin-console/src/components/display/EmptyState.tsx`
- Modify: `Platform/admin-console/src/app/App.tsx`
- Modify: `Platform/admin-console/src/styles/tokens.css`
- Modify: `Platform/admin-console/src/styles/global.css`
- Test: `Platform/admin-console/src/components/layout/AdminShell.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 左侧导航存在且支持分组
- 顶部栏存在身份区、角色徽章、刷新与退出
- 页面头支持标题、描述、操作区
- 移动端菜单按钮存在

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/components/layout/AdminShell.test.tsx
```

Expected:
- 失败，提示壳层组件不存在或语义结构不完整

**Step 3: 写最小实现**

- 落地极简企业 SaaS 视觉 tokens：
  - 颜色
  - 圆角
  - 间距
  - 阴影
  - 状态色
- 建立后台壳层：
  - 左侧导航
  - 顶部栏
  - 内容区
  - 全局状态/Toast 容器

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/components/layout/AdminShell.test.tsx
```

Expected:
- 壳层结构测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/components Platform/admin-console/src/styles Platform/admin-console/src/app
git commit -m "feat: add admin shell and design system foundations"
```

---

### Task 3: 建立 API Client、鉴权与权限驱动 UI 基础设施

**Files:**
- Create: `Platform/admin-console/src/services/http.ts`
- Create: `Platform/admin-console/src/services/adminApi.ts`
- Create: `Platform/admin-console/src/services/contracts.ts`
- Create: `Platform/admin-console/src/stores/uiStore.ts`
- Create: `Platform/admin-console/src/stores/sessionStore.ts`
- Create: `Platform/admin-console/src/hooks/useAdminSession.ts`
- Create: `Platform/admin-console/src/hooks/useCapabilities.ts`
- Create: `Platform/admin-console/src/hooks/useConfirmAction.ts`
- Create: `Platform/admin-console/src/components/feedback/ConfirmDialog.tsx`
- Create: `Platform/admin-console/src/schemas/admin.ts`
- Test: `Platform/admin-console/src/hooks/useCapabilities.test.tsx`
- Test: `Platform/admin-console/src/services/adminApi.test.ts`

**Step 1: 写失败测试**

测试覆盖：
- cookie session 模式请求默认 `credentials: include`
- 401/403 会被标准化为用户可见错误
- capability 判断能驱动导航/按钮显隐
- confirm dialog 可被统一调用

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/hooks/useCapabilities.test.tsx src/services/adminApi.test.ts
```

Expected:
- 失败，提示鉴权 client、capability hooks、confirm dialog 未实现

**Step 3: 写最小实现**

- 封装统一 `fetch` client
- 建立 `/admin/session`、`/admin/session/login`、`/admin/session/logout` 访问层
- 建立 capability 工具：
  - 模块访问
  - 读权限
  - 写权限
- 建立全局危险操作确认弹窗

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/hooks/useCapabilities.test.tsx src/services/adminApi.test.ts
```

Expected:
- 权限与 API 基础设施测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/services Platform/admin-console/src/stores Platform/admin-console/src/hooks Platform/admin-console/src/schemas Platform/admin-console/src/components/feedback
git commit -m "feat: add admin api client and permission-aware session layer"
```

---

### Task 4: 实现仪表盘页

**Files:**
- Create: `Platform/admin-console/src/pages/dashboard/DashboardPage.tsx`
- Create: `Platform/admin-console/src/pages/dashboard/dashboard.query.ts`
- Create: `Platform/admin-console/src/pages/dashboard/dashboard.types.ts`
- Create: `Platform/admin-console/src/components/charts/SimpleTrendPanel.tsx`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/dashboard/DashboardPage.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- KPI 卡片渲染
- 风险卡渲染
- 快捷入口可跳转
- 时间窗口切换可触发查询参数变化

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/dashboard/DashboardPage.test.tsx
```

Expected:
- 失败，提示 dashboard 页面未实现

**Step 3: 写最小实现**

- 基于现有 `/admin/dashboard` 或等效接口接入
- 渲染：
  - KPI 行
  - 趋势区
  - 风险区
  - 快捷入口
- 风险卡点击跳转到对应模块

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/dashboard/DashboardPage.test.tsx
```

Expected:
- Dashboard 页面测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/dashboard Platform/admin-console/src/components/charts Platform/admin-console/src/app/router.tsx
git commit -m "feat: build redesigned admin dashboard"
```

---

### Task 5: 实现用户中心与详情抽屉

**Files:**
- Create: `Platform/admin-console/src/pages/users/UsersPage.tsx`
- Create: `Platform/admin-console/src/pages/users/users.query.ts`
- Create: `Platform/admin-console/src/features/user-detail/UserDetailDrawer.tsx`
- Create: `Platform/admin-console/src/features/user-detail/UserOverviewTab.tsx`
- Create: `Platform/admin-console/src/features/user-detail/UserWalletTab.tsx`
- Create: `Platform/admin-console/src/features/user-detail/UserOrdersTab.tsx`
- Create: `Platform/admin-console/src/features/user-detail/UserAgreementsTab.tsx`
- Create: `Platform/admin-console/src/features/user-detail/UserUsageTab.tsx`
- Create: `Platform/admin-console/src/components/data/DataTable.tsx`
- Create: `Platform/admin-console/src/components/data/FilterBar.tsx`
- Create: `Platform/admin-console/src/components/data/DetailDrawer.tsx`
- Create: `Platform/admin-console/src/components/identity/UserIdentityCell.tsx`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/users/UsersPage.test.tsx`
- Test: `Platform/admin-console/src/features/user-detail/UserDetailDrawer.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 用户列表按用户名/用户编号/邮箱搜索
- 第一列优先展示用户名和编号，不主显 UUID
- 点击行打开详情抽屉
- 抽屉内可切换概览、钱包、订单、协议、用量标签

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/users/UsersPage.test.tsx src/features/user-detail/UserDetailDrawer.test.tsx
```

Expected:
- 失败，提示用户页与详情抽屉未实现

**Step 3: 写最小实现**

- 实现用户列表页
- 实现用户详情抽屉
- 按 capability 对各 tab 做只读/受限展示
- 详情页内保留“手动充值”“查看订单”等快捷动作入口

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/users/UsersPage.test.tsx src/features/user-detail/UserDetailDrawer.test.tsx
```

Expected:
- 用户页相关测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/users Platform/admin-console/src/features/user-detail Platform/admin-console/src/components/data Platform/admin-console/src/components/identity Platform/admin-console/src/app/router.tsx
git commit -m "feat: add user center with linked detail drawer"
```

---

### Task 6: 实现钱包与订单业务域

**Files:**
- Create: `Platform/admin-console/src/pages/wallet/WalletPage.tsx`
- Create: `Platform/admin-console/src/pages/orders/OrdersPage.tsx`
- Create: `Platform/admin-console/src/features/wallet-mutation/ManualRechargePanel.tsx`
- Create: `Platform/admin-console/src/features/wallet-mutation/WalletAdjustmentPanel.tsx`
- Create: `Platform/admin-console/src/features/orders/OrderDetailDrawer.tsx`
- Create: `Platform/admin-console/src/features/orders/ReconcileActionBar.tsx`
- Create: `Platform/admin-console/src/schemas/walletMutation.ts`
- Modify: `Platform/admin-console/src/services/adminApi.ts`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/wallet/WalletPage.test.tsx`
- Test: `Platform/admin-console/src/pages/orders/OrdersPage.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 钱包页可搜索用户并显示流水
- 手动充值与调账表单有 Zod 校验
- 危险操作弹窗显示对象、金额、说明
- 订单页支持状态筛选与对账动作

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/wallet/WalletPage.test.tsx src/pages/orders/OrdersPage.test.tsx
```

Expected:
- 失败，提示钱包页/订单页未实现

**Step 3: 写最小实现**

- 实现钱包列表、充值、调账页
- 实现订单列表与订单详情
- 把用户详情与钱包页做上下文联动
- 所有写操作复用统一确认流和 toast

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/wallet/WalletPage.test.tsx src/pages/orders/OrdersPage.test.tsx
```

Expected:
- 钱包与订单页面测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/wallet Platform/admin-console/src/pages/orders Platform/admin-console/src/features/wallet-mutation Platform/admin-console/src/features/orders Platform/admin-console/src/schemas/walletMutation.ts Platform/admin-console/src/services/adminApi.ts Platform/admin-console/src/app/router.tsx
git commit -m "feat: add wallet and orders workspace"
```

---

### Task 7: 实现模型与目录配置台

**Files:**
- Create: `Platform/admin-console/src/pages/catalog/CatalogModelsPage.tsx`
- Create: `Platform/admin-console/src/pages/catalog/CatalogRoutesPage.tsx`
- Create: `Platform/admin-console/src/pages/catalog/CatalogPricingPage.tsx`
- Create: `Platform/admin-console/src/pages/catalog/CatalogAgreementsPage.tsx`
- Create: `Platform/admin-console/src/features/catalog/ModelListPanel.tsx`
- Create: `Platform/admin-console/src/features/catalog/RouteEditorPanel.tsx`
- Create: `Platform/admin-console/src/features/catalog/PricingEditorPanel.tsx`
- Create: `Platform/admin-console/src/features/catalog/AgreementEditorPanel.tsx`
- Create: `Platform/admin-console/src/features/catalog/JsonAdvancedEditor.tsx`
- Create: `Platform/admin-console/src/schemas/catalog.ts`
- Modify: `Platform/admin-console/src/services/adminApi.ts`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/catalog/CatalogRoutesPage.test.tsx`
- Test: `Platform/admin-console/src/pages/catalog/CatalogPricingPage.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 路由编辑器默认协议是 `responses`
- 结构化编辑优先，JSON 编辑作为高级模式
- 定价规则显示版本、生效时间、价格
- 协议编辑器支持版本化编辑和发布前说明

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/catalog/CatalogRoutesPage.test.tsx src/pages/catalog/CatalogPricingPage.test.tsx
```

Expected:
- 失败，提示目录页未实现

**Step 3: 写最小实现**

- 把目录域拆成多页面或子路由
- 落地：
  - 官方模型
  - 路由
  - 定价
  - 协议
- 高级 JSON 编辑仅作为补充入口

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/catalog/CatalogRoutesPage.test.tsx src/pages/catalog/CatalogPricingPage.test.tsx
```

Expected:
- 目录配置相关测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/catalog Platform/admin-console/src/features/catalog Platform/admin-console/src/schemas/catalog.ts Platform/admin-console/src/services/adminApi.ts Platform/admin-console/src/app/router.tsx
git commit -m "feat: build catalog workspace for models routes pricing and agreements"
```

---

### Task 8: 实现审核与治理业务域

**Files:**
- Create: `Platform/admin-console/src/pages/governance/RefundsPage.tsx`
- Create: `Platform/admin-console/src/pages/governance/InfringementPage.tsx`
- Create: `Platform/admin-console/src/pages/governance/AuditsPage.tsx`
- Create: `Platform/admin-console/src/pages/governance/PoliciesPage.tsx`
- Create: `Platform/admin-console/src/features/governance/RefundDecisionPanel.tsx`
- Create: `Platform/admin-console/src/features/governance/InfringementDecisionPanel.tsx`
- Create: `Platform/admin-console/src/features/governance/AuditFilterPanel.tsx`
- Create: `Platform/admin-console/src/features/governance/PolicyEditorPanel.tsx`
- Modify: `Platform/admin-console/src/services/adminApi.ts`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/governance/RefundsPage.test.tsx`
- Test: `Platform/admin-console/src/pages/governance/InfringementPage.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 退款待审项优先展示
- 侵权详情支持证据与处理结果
- 审计页支持筛选与导出
- 治理类高危写操作使用统一确认流

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/governance/RefundsPage.test.tsx src/pages/governance/InfringementPage.test.tsx
```

Expected:
- 失败，提示治理页未实现

**Step 3: 写最小实现**

- 建立退款、侵权、审计、策略页面
- 强调待处理优先
- 所有审批/发布动作使用统一危险操作确认

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/governance/RefundsPage.test.tsx src/pages/governance/InfringementPage.test.tsx
```

Expected:
- 治理页测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/governance Platform/admin-console/src/features/governance Platform/admin-console/src/services/adminApi.ts Platform/admin-console/src/app/router.tsx
git commit -m "feat: add governance workspace for refunds audits and infringement"
```

---

### Task 9: 实现管理员与权限页，并统一全局交互

**Files:**
- Create: `Platform/admin-console/src/pages/operators/OperatorsPage.tsx`
- Create: `Platform/admin-console/src/features/operators/OperatorFormPanel.tsx`
- Create: `Platform/admin-console/src/features/operators/RoleCapabilityPreview.tsx`
- Create: `Platform/admin-console/src/components/navigation/PermissionGate.tsx`
- Modify: `Platform/admin-console/src/components/layout/AdminSidebar.tsx`
- Modify: `Platform/admin-console/src/components/layout/AdminTopbar.tsx`
- Modify: `Platform/admin-console/src/services/adminApi.ts`
- Modify: `Platform/admin-console/src/app/router.tsx`
- Test: `Platform/admin-console/src/pages/operators/OperatorsPage.test.tsx`

**Step 1: 写失败测试**

测试覆盖：
- 管理员列表支持编辑
- role capability 预览正确
- 无权限模块不显示
- 只读角色进入页面时只显示只读内容

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/operators/OperatorsPage.test.tsx
```

Expected:
- 失败，提示管理员页未实现

**Step 3: 写最小实现**

- 实现管理员页
- 抽离 `PermissionGate`
- 顶部栏与侧边导航根据 capability 自适应
- 超级管理员变更加入更强风险说明

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm test -- --runInBand src/pages/operators/OperatorsPage.test.tsx
```

Expected:
- 管理员页测试通过

**Step 5: Commit**

```bash
git add Platform/admin-console/src/pages/operators Platform/admin-console/src/features/operators Platform/admin-console/src/components/navigation Platform/admin-console/src/components/layout Platform/admin-console/src/services/adminApi.ts Platform/admin-console/src/app/router.tsx
git commit -m "feat: add operators workspace and permission gates"
```

---

### Task 10: 接入 Go 托管、替换旧后台入口并做回归

**Files:**
- Create: `Platform/internal/api/admin_assets_embed.go`
- Modify: `Platform/internal/api/server.go`
- Modify: `Platform/internal/api/admin_ui_test.go`
- Modify: `Platform/internal/api/server_test.go`
- Modify: `Platform/admin-console/package.json`
- Create: `Platform/admin-console/scripts/build-admin.mjs`
- Create: `Platform/admin-console/README.md`

**Step 1: 写失败测试**

测试覆盖：
- `/admin` 返回新版后台入口
- 静态资源可由 Go 服务托管
- 旧单文件关键业务断言迁移到新版构建产物校验

**Step 2: 运行测试确认失败**

Run:

```bash
cd Platform
go test ./internal/api -count=1
```

Expected:
- 失败，提示仍在服务旧 `admin_index.html` 或 embed 资源未接入

**Step 3: 写最小实现**

- 前端构建输出到固定目录（例如 `Platform/admin-console/dist`）
- Go 使用 `embed` 托管构建产物
- 更新 `/admin` 入口和静态资源处理
- 将旧后台 UI 回归测试迁移为：
  - 构建产物断言
  - Go 服务入口断言

**Step 4: 运行测试确认通过**

Run:

```bash
cd Platform/admin-console
npm run build
cd ..
go test ./internal/api -count=1
```

Expected:
- 新后台静态托管测试通过

**Step 5: Commit**

```bash
git add Platform/internal/api Platform/admin-console
git commit -m "feat: serve redesigned admin console from go backend"
```

---

### Task 11: 全量验收、收口旧后台并更新文档

**Files:**
- Modify: `docs/plans/2026-03-14-admin-backend-ui-implementation.md`
- Create: `docs/qa/2026-03-17-admin-console-redesign-acceptance.md`
- Modify: `Platform/README.md`
- Modify: `docs/release-windows-runbook.md`
- Modify: `docs/release-macos-runbook.md`
- Optional Delete/Archive: `Platform/internal/api/admin_index.html`（仅在新版完全接管且测试迁移完成后）

**Step 1: 写验收清单**

新增验收文档，覆盖：
- UI/UX
- 功能
- 权限
- 响应式
- 危险操作
- 回归测试

**Step 2: 运行前后端完整验证**

Run:

```bash
cd Platform/admin-console
npm test
npm run build
cd ..
go test ./... -count=1
```

Expected:
- 前端测试通过
- 构建成功
- Platform Go 测试通过

**Step 3: 手工验收**

手工验证角色：
- `super_admin`
- `operations`
- `finance`
- `governance`
- `read_only`

重点验证：
- 导航权限
- 用户详情抽屉
- 手动充值
- 调账
- 对账
- 路由/定价编辑
- 退款/侵权处理

**Step 4: 文档同步**

- 更新后台部署说明
- 更新运行手册
- 更新验收文档

**Step 5: Commit**

```bash
git add docs Platform/README.md
git commit -m "docs: add admin console redesign acceptance and rollout notes"
```

---

## 交付顺序建议

1. Task 1-3：基础设施
2. Task 4-6：核心业务（仪表盘、用户、钱包、订单）
3. Task 7-8：目录与治理
4. Task 9：管理员与权限
5. Task 10-11：接管、验收、收口

---

## 总体验收命令

```bash
cd Platform/admin-console
npm install
npm test
npm run build

cd ../..
cd Platform
go test ./... -count=1
```

---

## 风险提示

- `Platform/internal/api/admin_index.html` 当前承载大量业务逻辑；迁移时不要直接大删，先完成 React 前端接管后再移除
- 现有 `admin_ui_test.go` 依赖大量静态字符串断言，迁移时需要分批改造成“构建产物 + 行为测试”
- 危险操作体验要升级，但不能弱化后端权限校验与审计日志
- 旧数据字段（如 UUID / user_no / username）在新版身份展示中要统一映射，避免各页面再次分裂

---

Plan complete and saved to `docs/plans/2026-03-17-admin-console-redesign-plan.md`. Two execution options:

**1. Subagent-Driven (this session)** - 我在当前会话里按任务分发子代理执行并逐轮审查（推荐）

**2. Parallel Session (separate)** - 你新开会话，用 `executing-plans` 按这份计划批量推进

**你选哪种？**
