# macOS 跟进任务：扩展诊断与修复链路

目的：把当前 MVP（已在 Windows 侧跑通）补齐到 macOS 可发布水位。  
接手对象：后续 mac 开发 Agent。

---

## 1) 当前已完成（跨平台基础）

- `/plugins/status` 已输出：
  - `runtime_status`
  - `checks`
  - `repair_actions`
- Launcher 已有「扩展健康」诊断页（含审批弹窗）。
- `/plugins/repair` 已接通：
  - `install_node_deps`（可执行）
  - `install_bundled_cli`（审批后执行）
  - `explain_fix` / `set_env_path_hint`（说明模式）

说明：以上功能逻辑是通用实现，但尚未做 mac 实机回归。

---

## 2) mac 风险点（需重点验证）

1. **`npm install -g` 权限问题**
   - `install_bundled_cli` 当前执行 `npm install -g @clawdbot/lobster`
   - mac 常见失败：EACCES、global prefix 无写权限

2. **GUI 进程 PATH 问题**
   - Launcher 从 GUI 启动时，环境 PATH 常与终端不同
   - 安装后 `lobster --version` 可能在终端可用、在 Launcher 进程不可见

3. **安装后命令可见性**
   - `npm -g` 安装目录是否在进程 PATH
   - 是否需要显式补充 prefix/bin 到 PATH

4. **超时与网络回退**
   - `install_bundled_cli` 当前 3 分钟超时
   - 慢网环境可能触发误失败

---

## 3) mac 实机验证清单

在 mac 机器执行（建议 clean user 环境）：

1. 启动 Launcher + Gateway，打开「扩展健康」
2. 验证 lobster 缺失场景：
   - `runtime_status=degraded`
   - `checks` 含 `exe:lobster` 失败
3. 点击 `install_node_deps`：
   - 能执行并看到日志回显
4. 点击 `install_bundled_cli`：
   - 先返回 `needs_approval`
   - 确认后执行安装
5. 安装后自动刷新：
   - 目标状态 `degraded -> ready`（至少 `exe:lobster` 通过）
6. 重启 Launcher 再验证一次状态持久性

---

## 4) 建议最小改造（mac 优先）

1. **错误分类与文案增强**
   - 捕获 EACCES/permission denied，返回可读提示：
     - “npm 全局目录无写权限，请设置 npm prefix 或使用可写目录”

2. **可执行命令解析增强**
   - `install_bundled_cli` 验证阶段增加对 npm global bin 的探测
   - 必要时将探测到的路径加入子进程 PATH 再执行 `lobster --version`

3. **动作降级策略**
   - 若 `npm -g` 权限失败，自动回退为 `needs_approval + 手动修复指引`

---

## 5) 可选中期优化（非本次必做）

- 将 `install_bundled_cli` 从全局安装改为“应用私有目录安装”
- 对应 `LOBSTER_BIN` 持久化写入配置（避免依赖系统 PATH）

---

## 6) 相关代码入口

- Gateway 修复接口：
  - `PinchBot/pkg/gateway/pluginsrepair/handler.go`
- 运行时检查：
  - `PinchBot/pkg/plugins/runtime_status.go`
- 状态输出：
  - `PinchBot/pkg/plugins/plugin_settings.go`
- Launcher 诊断交互：
  - `Launcher/app-wails/frontend/index.html`
  - `Launcher/app-wails/app.go`

