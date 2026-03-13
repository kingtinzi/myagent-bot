# Git 协作规范（开发期）

适用范围：当前项目未上线，采用共享开发基线分支协作。

## 分支职责

- `main`: 里程碑分支，保持相对稳定，不承接日常并行开发。
- `integration/platform-refactor`: 当前共享开发基线，入口、登录、平台后端、官方模型、支付链路均以此为准。
- `feature/*`: 个人功能分支，从共享开发基线切出。

## 开发起点

禁止从旧 `main` 直接切平台相关功能分支。

统一起步命令：

```bash
git fetch origin
git switch -c feature/<name> origin/integration/platform-refactor
```

## 日常同步

每天开始开发前执行：

```bash
git fetch origin
git switch integration/platform-refactor
git pull --ff-only
git switch feature/<name>
git rebase integration/platform-refactor
```

## 合并顺序

1. `feature/*` -> `integration/platform-refactor`
2. 阶段验收通过后：`integration/platform-refactor` -> `main`

## 提交规范

推荐前缀：

- `feat:`
- `fix:`
- `refactor:`
- `test:`
- `docs:`
- `chore:`

示例：

```text
feat(wallet): add recharge history filters
fix(launcher): avoid duplicate platform startup
docs(release): clarify live platform config bootstrap
```

## 高冲突目录（改动前先同步）

- `Launcher/app-wails/app.go`
- `PinchBot/cmd/picoclaw-launcher/internal/server/*`
- `PinchBot/pkg/platformapi/*`
- `Platform/internal/api/*`
- `Platform/internal/service/*`
- `Platform/internal/store/pg/store.go`
- `Platform/internal/runtimeconfig/*`
- `scripts/*`

## 合并前最小验证

平台后端：

```bash
cd Platform
go test ./...
```

桥接与客户端：

```bash
cd PinchBot
go test ./pkg/platformapi ./cmd/picoclaw-launcher/internal/server
```

桌面入口（Windows 目标）：

```bash
cd Launcher/app-wails
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build ./...
```

## PR 描述要求

每个 PR 需明确：

- 影响模块
- 是否修改入口/启动链路
- 是否修改登录/支付/官方模型/数据库迁移
- 已执行验证命令与结果
- 是否引入配置变更

## 当前执行约定

如果同事要继续开发，统一基于：

```bash
origin/integration/platform-refactor
```

不要基于旧 `main` 开发平台化相关任务。
