# Windows 发布 Runbook

本文是给发布负责人用的 **Windows 实操清单**，目标是在真实 Windows 环境里把 PinchBot 做成可对外分发的安装包，并完成代码签名与干净环境验收。

## 1. 发布前准备

至少准备：

- Windows 10/11 发布机
- Go 工具链
- Inno Setup 6（如需安装器）
- 可用的代码签名证书
- `signtool.exe`
- 一套可用的 live 平台配置

平台聊天与官方模型相关说明：

- 只要你要交付登录后的聊天能力，`config/platform.env` 就不是可选项
- `launcher-chat.exe` 会从发布包根目录自动拉起 `platform-server.exe`
- 用户数据默认写入程序同目录下的 `.pinchbot/`

## 2. 构建发布包

在仓库根目录执行：

```powershell
.\scripts\build-release.ps1 -Version "1.0.0" -Zip -Installer
```

产物通常包括：

- `dist\PinchBot-<版本>-Windows-x86_64\`
- `dist\PinchBot-<版本>-Windows-x86_64.zip`
- `dist\PinchBot-<版本>-Windows-x86_64-Setup.exe`

安装器默认安装到：

```text
%LOCALAPPDATA%\Programs\PinchBot
```

## 3. 准备 live 配置

至少准备：

- `config\platform.env`

如果需要预置官方模型目录/路由/价格：

- `config\runtime-config.json`

建议方式：

1. 从 `config\platform.example.env` 复制出 `config\platform.env`
2. 填入真实 `PLATFORM_*`
3. 需要预置官方模型时，再从 `runtime-config.example.json` 复制出 `runtime-config.json`

## 4. 代码签名

推荐使用仓库内脚本：

```powershell
$env:WIN_SIGN_CERT_SHA1 = "你的证书指纹"
$env:WIN_SIGN_TIMESTAMP_URL = "http://timestamp.digicert.com"

.\scripts\sign-windows.ps1 `
  -PackageDir "dist\PinchBot-<版本>-Windows-x86_64" `
  -InstallerPath "dist\PinchBot-<版本>-Windows-x86_64-Setup.exe"
```

脚本内部会调用：

- `signtool.exe sign`
- `signtool.exe verify`
- `Get-AuthenticodeSignature`

签名参数默认使用：

- 文件摘要：`sha256`
- 时间戳摘要：`sha256`
- RFC3161 时间戳：`/tr`

如果你的 `signtool.exe` 不在 PATH，可设置：

```powershell
$env:WIN_SIGNTOOL_PATH = "C:\Program Files (x86)\Windows Kits\10\bin\...\signtool.exe"
```

## 5. 签名后验证

对关键文件逐个确认：

```powershell
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64\launcher-chat.exe"
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64\platform-server.exe"
Get-AuthenticodeSignature "dist\PinchBot-<版本>-Windows-x86_64-Setup.exe"
```

要求：

- `Status = Valid`

同时也可以执行：

```powershell
signtool verify /pa /v "dist\PinchBot-<版本>-Windows-x86_64\launcher-chat.exe"
```

## 6. 干净 Windows 验收

建议使用一台 **干净 Windows** 虚拟机或未装开发环境的物理机，重点检查：

1. 从 ZIP 解压，或运行 `PinchBot-<版本>-Windows-x86_64-Setup.exe`
2. 如果走安装器，确认安装目录为 `%LOCALAPPDATA%\Programs\PinchBot`
3. 双击 `launcher-chat.exe`
4. 确认 SmartScreen / Defender 不再因为未签名而弹出高风险拦截
5. 确认登录弹窗出现
6. 确认登录成功后可进入聊天界面
7. 确认“设置”页可打开
8. 确认官方模型可见
9. 确认钱包/充值协议可见
10. 如已配置支付，再确认支付跳转可用

同时检查：

- `launcher-chat.exe`
- `pinchbot.exe`
- `pinchbot-launcher.exe`
- `platform-server.exe`

是否按预期拉起与退出。

## 7. 最终交付建议

推荐交付以下之一：

- 已签名 ZIP
- 已签名安装器 `PinchBot-<版本>-Windows-x86_64-Setup.exe`
- 已签名目录包（仅限内部或可信交付场景）

## 8. 发布前检查表

- [ ] Windows 发布包构建完成
- [ ] live `config\platform.env` 已准备
- [ ] 如需预置，`config\runtime-config.json` 已准备
- [ ] `launcher-chat.exe` 已签名
- [ ] `pinchbot.exe` 已签名
- [ ] `pinchbot-launcher.exe` 已签名
- [ ] `platform-server.exe` 已签名
- [ ] 安装器 `PinchBot-<版本>-Windows-x86_64-Setup.exe` 已签名
- [ ] `Get-AuthenticodeSignature` 验证通过
- [ ] `signtool verify` 验证通过
- [ ] 干净 Windows 验收通过
- [ ] SmartScreen 行为已确认

## 不要做的事

- 不要把 `platform.example.env` 当作 live 配置直接发客户
- 不要在未签名状态下对外宣称 Windows 包已可正式商用分发
- 不要跳过干净 Windows 验收
