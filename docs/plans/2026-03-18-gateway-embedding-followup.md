# PinchBot Gateway 内嵌化第二阶段跟进计划

> Goal: 在第一阶段“launcher-chat 进程内托管 settings 服务”完成后，继续把 `pinchbot gateway` 从 CLI 入口中拆成可嵌入 service，最终把本地运行时收敛为**一个桌面主进程 + 一个远端 platform-server**。

## 当前边界梳理

### 现状调用链

- CLI 入口：`PinchBot/cmd/picoclaw/main.go`
- Gateway 命令：`PinchBot/cmd/picoclaw/internal/gateway/command.go`
- 真正启动逻辑：`PinchBot/cmd/picoclaw/internal/gateway/helpers.go`

当前 `gateway` 命令本质上只是：

1. Cobra 解析 `gateway --debug`
2. 调用 `gatewayCmd(debug)`
3. `gatewayCmd` 在一个函数里完成：
   - 加载配置
   - 初始化 provider / agent loop / bus
   - 初始化 cron / heartbeat / media store / channel manager / device service
   - 注册 `/health`、`/ready`、`/api/chat`
   - `signal.Notify(os.Interrupt)` 阻塞等待退出
   - 统一清理所有资源

### 目前不适合直接内嵌的点

1. **CLI 与服务生命周期耦合**
   - `gatewayCmd` 直接处理 `signal.Notify`
   - 内嵌到 launcher 后，桌面主进程不能把退出逻辑交给 `Ctrl+C`

2. **初始化、运行、清理没有显式分层**
   - 现在是一个“大函数”一次做完
   - 没有可复用的 `Start/Stop` 边界

3. **状态与依赖只存在局部变量里**
   - `provider`
   - `agentLoop`
   - `channelManager`
   - `mediaStore`
   - `cronService`
   - `heartbeatService`
   - `deviceService`
   - `usageLogger`
   - `stateManager`
   - `healthServer`

4. **桌面端需要的能力没有被抽象为可控接口**
   - 启动
   - 停止
   - 健康检查
   - 聊天 API
   - 日志桥接

## 第二阶段推荐方案

### 方案结论

新增一个**可嵌入 gateway service package**，由 CLI 和 launcher-chat 共同复用：

- CLI 继续保留：`pinchbot gateway`
- 桌面端新增：`launcher-chat` 直接在本进程内启动同一套 gateway service

推荐拆分为两层：

1. **组装层**
   - 负责加载 config、构建 provider、agent loop、channel manager 等依赖
2. **运行层**
   - 负责 `Start(ctx)` / `Stop(ctx)` / `ReadyURL()` / `ChatHandler()` / `HealthHandler()`

### 推荐目录

- Create: `PinchBot/pkg/gatewayservice/`
  - `service.go`
  - `builder.go`
  - `types.go`
  - `service_test.go`

CLI 包只保留很薄的一层：

- `cmd/picoclaw/internal/gateway/command.go`
- `cmd/picoclaw/internal/gateway/helpers.go`

其中 `helpers.go` 最终应降级为：

- 解析 debug flag
- 调用 `gatewayservice.New(...)`
- 监听 `os.Interrupt`
- 转发到 `svc.Start/Stop`

## 目标接口

### 最小公共接口

```go
type Service interface {
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    ReadyURL() string
    HealthURL() string
}
```

### 推荐构造参数

```go
type Options struct {
    ConfigPath string
    Debug bool
    Logger Logger
    ShutdownTimeout time.Duration
}
```

### 运行时状态建议

```go
type Runtime struct {
    Config *config.Config
    Provider providers.Provider
    AgentLoop *agent.AgentLoop
    ChannelManager *channels.Manager
    MediaStore media.MediaStore
    CronService *cron.CronService
    HeartbeatService *heartbeat.Service
    DeviceService *devices.Service
    UsageLogger *usage.Logger
}
```

## 分步拆分建议

### Step 1: 抽出“构建期”与“运行期”

把 `gatewayCmd(debug)` 拆为：

- `BuildRuntime(opts) (*Runtime, error)`
- `StartRuntime(ctx, rt) error`
- `StopRuntime(ctx, rt) error`

这样先不改变 CLI 行为，但先把大函数拆开。

### Step 2: 去掉服务内部的 `signal.Notify`

`signal.Notify` 只应保留在 CLI 包里。

内嵌模式下：

- launcher-chat 自己管理生命周期
- Wails `OnShutdown` 触发 `Service.Stop`

### Step 3: 收敛 HTTP server 控制权

当前 health / ready / `/api/chat` 是通过 `channelManager.SetupHTTPServer(...)` 挂载的。

需要明确：

- HTTP server 的启动时机
- 停止时机
- 监听地址
- ready 判定来源

建议让 `gatewayservice.Service` 成为 HTTP server 生命周期的唯一拥有者。

### Step 4: Launcher 改为本进程调用

在 `Launcher/app-wails/app.go` 中：

- `ensureGatewayServiceStarted()` 不再 `exec pinchbot.exe gateway`
- 改为：
  - 如果本进程内 gateway service 已启动，直接复用
  - 否则在当前进程内创建并启动 `gatewayservice.Service`

### Step 5: CLI 保持兼容

CLI 仍支持：

```bash
pinchbot gateway
```

只是内部改成复用 `gatewayservice.Service`。

## 验收标准

### 功能验收

- 不启动 `pinchbot.exe` 子进程时，桌面端仍可聊天
- `/health`、`/ready`、`/api/chat` 仍正常
- 本地附件上传、官方模型 session 校验、usage 记录不回退

### 生命周期验收

- launcher-chat 退出时，内嵌 gateway 能优雅关闭
- 不遗留僵尸进程
- 不再需要任务管理器里同时存在 `launcher-chat.exe` + `pinchbot.exe`

### 兼容验收

- `pinchbot gateway` 命令仍可单独运行
- 独立命令行开发/排查链路不受影响

## 风险清单

1. **依赖组装过多，拆分时容易漏清理**
   - 必须给 `Stop()` 明确顺序：
     - cancel context
     - stop channels
     - stop device / heartbeat / cron
     - stop media cleanup
     - stop agent loop
     - close usage logger
     - close provider

2. **HTTP server 所有权不清晰**
   - 需要避免 launcher 与 channelManager 双重持有/双重关闭

3. **日志与 readiness 事件可能错位**
   - 启动完成标准应以 `/ready` 为准，而不是仅凭“进程已启动”

4. **Windows 桌面环境下的关闭时序**
   - Wails `OnShutdown` 必须等待 gateway service 释放端口

## 推荐验证命令

### 现阶段分析基线

```bash
cd PinchBot
go test ./cmd/picoclaw/internal/gateway/... ./pkg/channels/...
```

### 第二阶段实施后

```bash
cd Launcher/app-wails
go test ./...

cd PinchBot
go test ./...
```

## 结论

第二阶段不建议“直接把 gateway 逻辑塞进 launcher app.go”。最稳妥的方式是先把 `gatewayCmd` 拆成**可复用 service**，再让 CLI 与桌面端共享同一实现。这样既能实现最终的本地单进程目标，也不会破坏现有命令行与开发调试链路。
