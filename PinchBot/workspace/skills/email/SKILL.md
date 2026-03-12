---
name: email
description: "使用 Email MCP 发邮件、发文件给用户或目标邮箱。依赖已配置的 MCP 服务器 email（@codefuturist/email-mcp）。"
metadata: {}
---

# Email 发信能力

**重要：当用户要求发邮件时，必须立即调用发邮件工具（工具名：mcp_email_send_email），不得仅用文字回复「我会发」或描述邮件内容。必须实际发起工具调用。**

当用户要求「用邮箱发文件」「发邮件给我/给某某」「用机器人的邮箱发一封邮件」时，使用 **email** MCP 提供的 **mcp_email_send_email** 完成。

## 前置条件

- 在 PinchBot 的 `config.json` 里已启用并配置 **email** MCP 服务器（见下方安装说明）。
- 已为该机器配置好专用邮箱（SMTP/IMAP），并完成账号验证。

## 主要工具（来自 email MCP）

| 场景           | 工具           | 说明 |
|----------------|----------------|------|
| 发新邮件       | `mcp_email_send_email` | 发一封新邮件（正文、HTML、CC/BCC；是否支持附件以工具参数为准） |
| 发已有草稿     | `mcp_email_send_draft` | 发送已保存的草稿 |
| 查有哪些账号   | `mcp_email_list_accounts` | 列出已配置的邮箱账号 |
| 回复/转发      | `mcp_email_reply_email` / `mcp_email_forward_email` | 按需使用 |

用户说「发文件」时：若 MCP 的 `send_email` 支持附件参数，则直接带附件发送；否则可先说明「当前仅支持正文」，或先上传到别处再发链接。

## 使用要点

1. **先确认账号**：需要时先调用 `list_accounts`，确认发件人账号名称/邮箱。
2. **收件人**：
   - 用户说「发给我」「发到我的邮箱」「发给主人」时，**必须优先使用 workspace/USER.md 中记录的当前用户/主人邮箱**（在「DingTalk (当前对话用户)」或类似章节下的 Email 字段）。USER.md 会随系统上下文加载，直接使用其中记录的邮箱，无需再向用户确认。
   - 若用户明确说了「发给 xxx@yy.com」，则使用该地址。
   - 若 USER.md 中无对应用户邮箱记录，再向用户确认收件地址。
3. **主题与正文**：根据用户意图填写主题和正文；若用户给了文件路径，在支持附件的前提下把文件作为附件带上。
4. **多账号**：若配置了多个账号，按用户指定或默认账号选择发件账号（工具参数通常有 account 或 from 等）。

## 安装与配置（管理员）

1. **安装**：需 Node.js ≥ 22。MCP 通过 npx 运行，无需全局安装：
   ```bash
   npx @codefuturist/email-mcp setup
   ```
   按提示添加邮箱账号（会写入 `~/.config/email-mcp/config.toml`）。

2. **或在 config 里用环境变量单账号**：在 `config.json` 的 `tools.mcp.servers.email` 中设置：
   - `enabled: true`
   - `command`: `npx`
   - `args`: `["-y", "@codefuturist/email-mcp", "stdio"]`
   - `env`: 填写 `MCP_EMAIL_ADDRESS`、`MCP_EMAIL_PASSWORD`、`MCP_EMAIL_IMAP_HOST`、`MCP_EMAIL_SMTP_HOST`（密码建议用环境变量或密钥管理，不要提交到仓库）。

3. **注册/申请邮箱**：为机器人单独注册一个邮箱（如 Gmail、QQ 邮箱、163、企业邮箱），并使用「应用专用密码」或 SMTP/IMAP 授权码，不要用主密码。

配置完成后重启 PinchBot，agent 即可通过本 SKILL 使用 email MCP 发邮件、发文件。
