# macOS 发布 Runbook

本文是给发布负责人用的 **实操清单**，目标是在真实 macOS 环境里把 `launcher-chat.app` 做成可对外分发的安装包。

与当前仓库实现强相关的前提：

- `launcher-chat.app` 内置的桌面聊天窗口本身就是登录后使用的流程。
- 因此只要你要交付桌面聊天能力，`config/platform.env` 就不是可选项。
- 桌面入口会从发布包根目录自动拉起 `platform-server`，所以 live 配置必须放在发布包根目录的 `config/` 下，而不是 `.app/Contents/MacOS/` 里。
- 用户数据默认写入发布包同级目录的 `.pinchbot/`。
- `pinchbot-launcher` 只在点击“设置”时按需启动，不会默认常驻。

相关实现位置：

- `Launcher/app-wails/app.go`
- `scripts/build-release.sh`
- `docs/build-and-release.md`

## 1. 发布前准备

在真实 Mac 上准备以下内容：

- Xcode Command Line Tools
- Go 工具链
- Apple Developer `Developer ID Application` 证书
- notarization 凭据
- 一套可用的生产或准生产平台配置

建议先确认：

- `PLATFORM_SUPABASE_*`
- `PLATFORM_DATABASE_URL`
- `PLATFORM_PAYMENT_PROVIDER`
- `PLATFORM_EASYPAY_*`
- `PLATFORM_RUNTIME_CONFIG_PATH` 是否按默认路径使用

## 2. 准备 live 配置

发布包里默认只会带：

- `config/platform.example.env`
- `config/runtime-config.example.json`

它们只是示例，不会自动变成 live 配置。

正式发布前，至少准备：

- `config/platform.env`

如果你希望包里直接带初始官方模型/价格/协议，再准备：

- `config/runtime-config.json`

推荐做法：

1. 先从 `config/platform.example.env` 复制出 `config/platform.env`
2. 填入真实环境变量
3. 如需预置官方模型，再从 `runtime-config.example.json` 复制出 `runtime-config.json`

## 3. 在 Mac 上构建

在仓库根目录执行：

```bash
export VERSION=1.0.0
export MAC_CODESIGN_IDENTITY="Developer ID Application: Your Company (TEAMID)"

chmod +x scripts/build-release.sh
./scripts/build-release.sh "$VERSION" -z
```

构建结果默认在：

```text
dist/PinchBot-$VERSION-Darwin-arm64/
```

或 Intel 机器上的：

```text
dist/PinchBot-$VERSION-Darwin-amd64/
```

## 4. 检查发布目录

确认目录结构至少包含：

```text
PinchBot-<version>-Darwin-<arch>/
  launcher-chat.app/
  config/
  README.txt
```

确认 `.app` 内包含：

```text
launcher-chat.app/Contents/MacOS/
  launcher-chat
  pinchbot-launcher
  pinchbot
  platform-server
```

## 5. 把 live 配置放进包根目录

要注意路径必须是：

```text
dist/PinchBot-<version>-Darwin-<arch>/config/platform.env
```

不是：

```text
dist/.../launcher-chat.app/Contents/MacOS/config/platform.env
```

如果你还要预置 runtime config，则路径是：

```text
dist/PinchBot-<version>-Darwin-<arch>/config/runtime-config.json
```

## 6. 本机 smoke test

在当前 Mac 上直接用发布产物做一次完整冒烟：

1. 双击 `launcher-chat.app`
2. 确认应用能正常打开
3. 确认登录/注册弹窗出现
4. 使用真实或测试账号登录
5. 确认登录后可以进入聊天界面
6. 确认设置页能打开
7. 确认官方模型列表可用
8. 确认充值协议可见
9. 如果支付环境已配置，确认支付跳转正常

同时检查自动拉起/按需启动的后台进程是否正常：

- `pinchbot-launcher`
- `pinchbot`
- `platform-server`

如果登录后聊天仍不可用，第一优先检查：

- 包根目录下是否真的存在 `config/platform.env`
- 里面的 `PLATFORM_SUPABASE_*` 是否正确
- 数据库与支付配置是否可达

## 7. 验证签名

如果构建时已经设置了 `MAC_CODESIGN_IDENTITY`，继续执行：

```bash
codesign --verify --deep --strict --verbose=2 "dist/PinchBot-$VERSION-Darwin-arm64/launcher-chat.app"
spctl --assess --type execute -vv "dist/PinchBot-$VERSION-Darwin-arm64/launcher-chat.app"
```

如果这里失败，不要继续 notarization，先修签名问题。

## 8. 做 notarization

建议使用 `notarytool`。

先准备认证信息，然后提交：

```bash
ditto -c -k --keepParent \
  "dist/PinchBot-$VERSION-Darwin-arm64/launcher-chat.app" \
  "dist/launcher-chat-$VERSION.zip"

xcrun notarytool submit "dist/launcher-chat-$VERSION.zip" \
  --keychain-profile "notarytool-password" \
  --wait
```

通过后执行：

```bash
xcrun stapler staple "dist/PinchBot-$VERSION-Darwin-arm64/launcher-chat.app"
spctl --assess --type execute -vv "dist/PinchBot-$VERSION-Darwin-arm64/launcher-chat.app"
```

## 9. 干净环境验收

正式对外前，建议换一台没有开发证书、没有本地开发环境残留的 Mac 再测一次：

1. 从最终交付物解压
2. 双击 `launcher-chat.app`
3. 确认 Gatekeeper 不再拦截
4. 确认登录可用
5. 确认登录后的聊天可用
6. 确认官方模型/钱包/充值链路可用

## 10. 最终交付建议

推荐交付以下内容之一：

- 已 notarize 的完整目录
- 已 notarize 的压缩包

同时附带：

- 客户版 `README.txt`
- 需要客户填写或替换的配置项清单
- 支付/平台环境接入说明

## 发布前最终检查表

- [ ] 在真实 macOS 上构建完成
- [ ] `launcher-chat.app` 结构完整
- [ ] 包根目录下存在 live `config/platform.env`
- [ ] 如需预置，`config/runtime-config.json` 已准备
- [ ] 登录/注册流程可用
- [ ] 登录后的聊天可用
- [ ] 官方模型可用
- [ ] 充值协议可见
- [ ] 支付跳转可用
- [ ] `codesign --verify` 通过
- [ ] `spctl --assess` 通过
- [ ] notarization 通过
- [ ] `stapler` 已执行
- [ ] 干净 Mac 验收通过

## 不要做的事

- 不要把 `platform.example.env` 当成 live 配置直接发客户
- 不要把 live 配置放到 `.app/Contents/MacOS/config/`
- 不要在 Windows 主机上把 `build-release.sh` 的结果当成 mac 验证通过
- 不要在未签名、未 notarize 的情况下对外宣称 mac 包已可正式分发
