# PinchBot 硬切命名重构设计

日期：2026-03-14

## 背景

当前仓库虽然已将用户可见品牌大部分收敛为 PinchBot，但运行时代码、模块路径、迁移体系和部分测试中仍保留 `OpenClaw/openclaw` 残留。这些残留会持续污染团队协作、部署配置与后续重构判断。

项目仍处于开发阶段、尚未正式上线，因此本次采取**彻底切断旧兼容**策略，而不是继续维持双命名兼容。

## 目标

1. 运行时代码只接受 `PinchBot` 命名。
2. 删除旧 `OpenClaw` 运行兼容：cookie、环境变量、更新目录、迁移入口。
3. 平台与桌面子模块的 Go module/import 路径统一切换到 `pinchbot/...`。
4. 删除 `openclaw` 迁移源和相关测试、命令帮助、文档。
5. 最终做到：**运行时代码中 `openclaw/OPENCLAW` 残留为 0**；只允许极少数必要的历史说明文本存在，如无必要也一并删除。

## 已评估方案

### 方案 A：全量硬切 `OpenClaw -> PinchBot`

- 改 module/import、cookie、env、更新目录、脚本、文档和测试。
- 删除 `openclaw` 迁移源与旧兼容逻辑。
- 优点：一次清债，后续维护最干净。
- 缺点：改动面较大，需要严格回归测试。

### 方案 B：运行时硬切，内部命名暂留

- 只改用户可见和运行边界，内部 `openclaw/...` 暂保留。
- 优点：风险较低。
- 缺点：不符合“彻底切断旧兼容”的要求，后续仍会返工。

### 方案 C：新建 `pinchbot/*`，废弃旧 `openclaw/*`

- 通过新目录/新模块再迁移实现。
- 优点：视觉上最整齐。
- 缺点：改动和冲突面最大，不适合当前已有大量在制代码的仓库。

## 最终决策

采用 **方案 A：全量硬切**。

## 设计范围

### 1. 运行时边界硬切

- `openclaw_admin_session` → `pinchbot_admin_session`
- `OPENCLAW_*` → `PINCHBOT_*`
- 更新目录、缓存目录、脚本目录中的 `OpenClaw` → `PinchBot`
- 删除旧 fallback 逻辑，不再识别旧变量/旧目录

### 2. Go 模块与 import 硬切

- `Platform/go.mod` → `pinchbot/platform`
- `Launcher/app-wails/go.mod` → `pinchbot/launcher/app-wails`
- 全仓 import 批量收敛到新 module path
- 所有测试、工具脚本、文档引用同步更新

### 3. 迁移体系硬切

- 删除 `PinchBot migrate --from openclaw`
- 删除 `PinchBot/pkg/migrate/sources/openclaw/*`
- 删除 `.openclaw`、`openclaw.json`、`OPENCLAW_HOME` 相关逻辑与测试
- 仅保留 PinchBot 当前配置体系

### 4. 术语残留与回归验证

- 清理注释、测试文案、README、发布文档里的 `OpenClaw`
- 对运行时代码进行全仓扫描，确保 `openclaw/OPENCLAW` 不再出现

## 实施波次

### Wave 1：运行时边界硬切

先修改 cookie、环境变量、更新目录、默认路径与对应测试，确保真实运行边界不再接受旧命名。

### Wave 2：Go 模块与 import 硬切

统一修改 `go.mod` 与 import 路径，并跑平台端/支付/后台/桌面相关测试。

### Wave 3：迁移体系硬切

删除 `openclaw` 迁移源、CLI 帮助、相关测试与配置路径探测。

### Wave 4：术语清扫与验收

做全仓扫描、修正文档与注释，最终只留下 PinchBot。

## 切断后的行为定义

1. 旧 `OpenClaw` 管理后台 cookie 直接失效。
2. 旧 `OPENCLAW_*` 环境变量完全不再读取。
3. 旧 `OpenClaw/pending` 更新缓存完全不再识别。
4. 旧 `.openclaw` / `openclaw.json` 完全不再参与启动或迁移。
5. `PinchBot migrate --from openclaw` 删除；若未来确有历史迁移需求，应单独提供离线迁移工具，而不是在主分支中保留兼容逻辑。

## 风险与控制

### 风险

- Go module/import 硬切会影响多处编译入口。
- 迁移代码删除可能影响旧测试和历史工具入口。
- 当前工作树存在大量未提交修改，需避免覆盖无关在制工作。

### 控制措施

- 按 Wave 分阶段修改，每一波完成后执行对应测试。
- 仅修改与本次命名硬切直接相关的文件。
- 若发现与其他未提交工作直接冲突，则停止并显式报告，不进行覆盖。

## 验收标准

1. 运行时代码中 `openclaw` / `OPENCLAW` 残留为 0。
2. Platform、Launcher、支付、后台相关测试通过。
3. 新环境只识别 `PinchBot` 命名，不再读取旧兼容标识。
4. 文档、测试、脚本、CLI 帮助不再将 `OpenClaw` 作为当前产品名或有效运行输入。
