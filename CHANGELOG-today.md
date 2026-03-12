# 今日修改摘要 (Summary of today's changes)

## 项目重命名与目录结构调整
- **PicoClaw → PinchBot**：目录与品牌统一为 PinchBot
- **数据目录**：配置与工作区改为「程序同目录」的 `.pinchbot`（exe_dir/.pinchbot）
  - 首次运行自动创建 .pinchbot 与默认 config.json（workspace: "workspace"）
  - 网关启动时若工作区目录不存在则自动创建模板
  - 支持环境变量 PINCHBOT_HOME / PINCHBOT_CONFIG 覆盖

## 构建与发布
- **build-release.ps1**：不再从 %USERPROFILE%\.pinchbot 复制到 dist；仅构建 exe + README，首次运行由程序生成 .pinchbot

## Launcher-chat (Wails 托盘 + 聊天窗)
- **默认只启动网关**：不再自动启动 PinchBot-launcher；点击「设置」时按需启动配置页
- **网关日志**：本机缓存网关 stdout/stderr，打开设置时推送到配置页，日志与当前运行的 PinchBot 绑定
- **窗口**：恢复系统标题栏便于拖动；删除自定义标题栏（红框内的标题/设置/关闭）
- **退出**：托盘「退出」时先结束子进程并 Wait，再退出主进程，便于退出后删除 dist 目录

## PinchBot-launcher（配置页）
- **网关状态**：仅检测并显示 PinchBot 是否在运行，移除「启动」按钮；保留「停止」按钮
- **日志**：支持 launcher-chat 通过 POST /api/process/logs/start 与 /api/process/logs 推送网关输出，配置页展示当前运行网关的日志
- **从配置页启动网关**：若已在运行则直接返回成功；启动时设置 cmd.Dir 为 exe 目录；未找到 PinchBot.exe 时返回明确错误

## 工作区配置
- **后端**：相对路径（如 "workspace"）解析为 GetPinchBotHome()/workspace（.pinchbot/workspace）；新增 GetPinchBotHome()
- **配置页**：工作区目录默认/占位为 "workspace"；说明与提示更新为「数据与程序同目录、推荐相对路径 workspace」

## 安全与说明
- 网关与配置页默认监听 127.0.0.1，不暴露于外网
- 钉钉通道为出站连接（Stream），无需开放本机端口
