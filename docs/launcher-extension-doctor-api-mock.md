# Launcher 扩展诊断页 - API Mock 与组件拆分

用于前后端并行开发与联调。

---

## 1. 组件拆分（前端）

建议文件：

- `Launcher/app-wails/frontend/src/extensions/ExtensionDoctorPage.tsx`
- `Launcher/app-wails/frontend/src/extensions/ExtensionCard.tsx`
- `Launcher/app-wails/frontend/src/extensions/ExtensionDetailDrawer.tsx`
- `Launcher/app-wails/frontend/src/extensions/RepairConfirmModal.tsx`
- `Launcher/app-wails/frontend/src/extensions/api.ts`

组件职责：

- `ExtensionDoctorPage`
  - 拉取列表
  - 过滤、搜索、分页
  - 轮询刷新
- `ExtensionCard`
  - 展示状态和快捷修复按钮
- `ExtensionDetailDrawer`
  - 展示 `checks`、`reason_code`、技术细节
- `RepairConfirmModal`
  - `requires_approval=true` 的确认动作

---

## 2. `/plugins/status` mock

```json
{
  "node_host": true,
  "plugins_enabled": ["lobster", "openclaw-lark"],
  "extensions_dir": "D:/ProgramData/OpenClaw/extensions",
  "plugins": [
    {
      "plugin_id": "lobster",
      "name": "Lobster",
      "runtime_status": "degraded",
      "checks": [
        { "id": "node", "ok": true, "detail": "v22.12.0", "reason_code": "" },
        { "id": "npm_deps", "ok": true, "detail": "node_modules ready", "reason_code": "" },
        { "id": "exe:lobster", "ok": false, "detail": "not found in env/bundled/PATH", "reason_code": "BIN_NOT_FOUND" }
      ],
      "repair_actions": [
        {
          "id": "install_bundled_cli",
          "label": "安装内置 Lobster 运行时",
          "risk": "medium",
          "requires_approval": true
        },
        {
          "id": "set_env_path_hint",
          "label": "查看手动路径配置指引",
          "risk": "low",
          "requires_approval": false
        }
      ]
    },
    {
      "plugin_id": "openclaw-lark",
      "name": "OpenClaw Lark",
      "runtime_status": "ready",
      "checks": [
        { "id": "node", "ok": true, "detail": "v22.12.0", "reason_code": "" }
      ],
      "repair_actions": []
    }
  ]
}
```

---

## 3. `POST /plugins/repair` mock

请求：

```json
{
  "plugin_id": "lobster",
  "action_id": "install_bundled_cli",
  "args": {}
}
```

响应（需要确认）：

```json
{
  "ok": true,
  "status": "needs_approval",
  "approval": {
    "token": "repair-approval-abc",
    "title": "确认安装扩展运行时",
    "message": "将安装 lobster 到应用私有目录，不修改系统 PATH。"
  }
}
```

响应（执行成功）：

```json
{
  "ok": true,
  "status": "ok",
  "result": {
    "plugin_id": "lobster",
    "action_id": "install_bundled_cli",
    "duration_ms": 2140,
    "logs": [
      "downloaded bundle",
      "installed to D:/ProgramData/OpenClaw/tools/lobster",
      "validated lobster --version"
    ]
  }
}
```

响应（失败）：

```json
{
  "ok": false,
  "status": "failed",
  "error": {
    "code": "NETWORK_ERROR",
    "message": "下载运行时失败，请检查网络后重试。"
  }
}
```

---

## 4. 前端状态机

- `idle`
- `loading_status`
- `status_ready`
- `repair_submitting`
- `repair_needs_approval`
- `repair_running`
- `repair_done`
- `repair_failed`

转移规则：

- 打开页面：`idle -> loading_status -> status_ready`
- 点击修复：
  - 无确认：`status_ready -> repair_submitting -> repair_running`
  - 有确认：`status_ready -> repair_submitting -> repair_needs_approval -> repair_running`
- 修复结束：`repair_running -> repair_done -> loading_status`

---

## 5. UI 文案键（i18n 建议）

- `extDoctor.title` = `扩展诊断`
- `extDoctor.status.ready` = `就绪`
- `extDoctor.status.degraded` = `可用但受限`
- `extDoctor.status.blocked` = `不可用`
- `extDoctor.action.repair` = `一键修复`
- `extDoctor.action.viewDetail` = `查看详情`
- `extDoctor.toast.repairSuccess` = `修复成功，扩展已恢复就绪`
- `extDoctor.toast.repairFailed` = `修复失败，请查看详情`

---

## 6. 联调 Checklist

- [ ] 使用 mock 能完整跑通“查看状态 -> 修复 -> 刷新状态”
- [ ] 后端字段缺失时前端不崩溃
- [ ] `requires_approval` 动作必须弹确认
- [ ] 修复进行中按钮禁用，避免重复提交
- [ ] 失败消息优先显示后端 `error.message`

