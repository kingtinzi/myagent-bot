# 聊天窗口：右侧入口 + 内嵌网页

## 思路

1. **右侧下面有入口**：任务栏右侧（托盘）放一个小图标，点击后弹出聊天窗口；关闭窗口只隐藏不退出，再从托盘可打开。
2. **内部是网页**：窗口里不画原生控件，而是**直接内嵌一个网页**（WebView），所以整个聊天界面就是前端在做。
3. **直接嵌套**：用 **Wails** 做壳，主窗口就是一个 WebView，加载本地打包进去的 `frontend/` 静态资源（或开发时连本地 dev server）。
4. **找开源前端再改**：`frontend/` 里用现成的开源聊天 UI（Vue/React/纯 HTML 均可），只改样式、接口和业务逻辑，比从零写快很多。

## 技术选型

| 层级     | 方案        | 说明 |
|----------|-------------|------|
| 壳       | Wails v2     | Go + WebView2（Windows），托盘 + 单窗口，Bind 暴露 Go 给 JS |
| 内嵌内容 | 静态前端    | 可打包进二进制（embed），或开发时用 Vite 等 |
| 聊天 UI  | 开源改一版  | 见下 |

## 可选开源聊天前端（替换 frontend 用）

- **[vue-advanced-chat](https://github.com/advanced-chat/vue-advanced-chat)** — Vue 聊天组件，支持流式、多媒体、主题，后端无关，适合对接自建 API。
- **[chat-ui](https://github.com/nvima/chat-ui)** — 多框架可用的聊天 UI，可定制。
- **[Nuxt Ollama Chat](https://github.com/apoStyLEE/nuxt-ollama-chat)** — 极简、流式，若用 Nuxt 可参考其样式与 API 对接方式。
- 任意「Ollama / OpenAI 风格」的 HTML+Vue/React 模板，把请求地址改成 Launcher 提供的接口（如通过 `window.go.Chat(text)` 调 Go，Go 再请求 Gateway 或本地 agent）。

## 数据流（示例）

- 用户在前端输入 → 调用 `window.go.Chat(message)`（Wails Bind）→ Go 里请求本地 Gateway 或 PinchBot → 流式/非流式结果通过回调或事件回给前端展示。
- 设置：可继续用「打开浏览器 18800」或在前端里嵌一个 iframe 加载 Launcher 配置页。

## 与主程序关系

- 不修改 PinchBot；Launcher 只通过 HTTP/本地调用与 Gateway 或 agent 通信。
- 配置（模型、MCP、权限等）仍在独立 `launcher.json` 或主 config，由配置页（18800）或后续在壳里提供 API 给前端读。
