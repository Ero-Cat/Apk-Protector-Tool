# 加固流程交互化：GUI / TUI 选型评估与设计

> 背景：`protector` 当前有 **28 个扁平 flag**，快速开始示例需要 14 个参数，无预设、无交互模式。本文评估三种交互化方案并给出落地设计。对应路线图条目 [P0.1 / P0.2](../ROADMAP.md)。

## 1. 问题陈述

一次完整加固，运维需要理解并正确组合：

- 6 个核心参数（输入/输出/报告/配置/工作目录/扫描开关）
- 8 个保护参数（总开关、单/多 DEX、压缩、包名随机化、密钥、伪加固、前缀）
- 10 个签名对齐参数（zipalign/apksigner/keytool 路径、keystore、两道密码、别名、验签、追加参数）
- 4 个第三方加固参数（命令、输出、参数、环境变量）

痛点实测（见 `README.md` 快速开始示例与 `cmd/protector/main.go:30-63`）：

1. **记忆负担**：路径类参数（build-tools 版本号因机而异）无法复用；
2. **安全暴露**：密钥只能明文进命令行，落入 shell history 与进程列表；
3. **分层分裂**：部分选项只能走配置文件（`keep_work_dir`、`reinforce.timeout` 等），心智模型两套；
4. **单向覆盖**：flag 只能开不能关，调试时只能去改配置文件。

## 2. 三方案对比

| 维度 | A. 零依赖向导（stdlib） | B. Bubbletea TUI ✅ | C. 本地 Web GUI |
|------|------------------------|--------------------|-----------------|
| 实现载体 | `protector -i` 问答式 stdin/stdout | `protector ui` 终端全屏应用 | 内嵌 HTTP 服务 + 浏览器页面 |
| 新增依赖 | 无 | `charmbracelet/bubbletea` + `bubbles`（约 10+ 间接模块） | net/http 自带 + 前端资源（embed），或再引 Wails 级框架 |
| 开发量 | S（2–3 天） | M（1–2 周） | L（3 周+） |
| 交互质量 | 线性问答，无法回退修改 | 表单/单选/多列选择/实时预览，Tab 导航 | 最丰富，表单校验与拖拽皆可 |
| 运维场景契合 | SSH/CI 均可用 | SSH 可用；CI 不可（需降级） | 需端口与浏览器，远程服务器需隧道 |
| headless/CI 兼容 | 好（可管道喂答案） | 需并行提供 `-profile`（本计划已含） | 差（本质是交互服务） |
| 安全面 | 密钥走 env 名，不过网络 | 同左，密钥留在进程内存 | **keystore 密码经浏览器 DOM/网络栈**，监听端口引入攻击面 |
| 与现有架构整合 | 直接复用 `LoadConfig`/`Tool.Run` | 同左 | 需新增服务层与状态管理 |

## 3. 结论

**选 B：Bubbletea TUI（`protector ui`）+ headless `-profile` 预设**，理由：

1. 目标用户是终端里的发布/运维工程师，SSH 场景占绝对多数——Web GUI 的浏览器要求反而是负担；
2. 密钥安全模型干净：密码只进进程内存，不经过浏览器与本地端口；
3. 配置生成物（config.json）天然可回灌：TUI 是"配置生成器 + 执行器"，产物进版本库即团队复用，比一次性 GUI 会话更可审计；
4. 代价（依赖树、开发量）可通过 P0.2 的 `-profile` 预设分摊——预设同时服务 CI 与熟练用户，TUI 只服务首次/低频用户。

不选 A 的原因：线性问答无法回退、无法预览生成的配置，对 28 个可选项的展开能力天花板太低。
不选 C 的原因：安全面（密钥过浏览器）与运维场景（SSH + CI）双重错配，开发量还最大。

## 4. Bubbletea TUI 设计

### 4.1 Screen 流

```
┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐    ┌────────────┐
│ S1 欢迎/选  │    │ S2 预设选择 │    │ S3 表单填写 │    │ S4 预览确认 │    │ S5 执行/结果 │
│ 择输入 APK │ ─▶ │ quick/full │ ─▶ │ 仅显示所选 │ ─▶ │ config diff│ ─▶ │ 进度条+扫描 │
│ (路径补全)  │    │ /sign-only │    │ 预设的差异项│    │ +安全检查   │    │ 结果+报告路径│
└────────────┘    └────────────┘    └────────────┘    └────────────┘    └────────────┘
       ▲                                                       │
       └───────────────── Esc 任意回退 ◀────────────────────────┘
```

- **S1**：输入 APK 路径（文件存在性即时校验 + 上次使用记忆 `~/.config/protector/state.json`）；
- **S2**：预设三选一（quick / full / sign-only），右侧面板实时展示该预设将启用的步骤；
- **S3**：按预设裁剪表单——`quick` 只问 keystore 四项；`full` 追加保护与密钥；所有路径字段带 `~` 展开与存在性校验，build-tools 路径自动探测 `~/Library/Android/sdk/build-tools/*/` 与 `$ANDROID_HOME`；
- **S4**：渲染最终 config.json（语法高亮 diff 对比预设基线）+ 安全校验（密钥是否将明文落盘、输出目录是否在 git 内）；
- **S5**：执行流水线，逐步显示 scan→protect→align→sign 状态，完成后给出产物路径、SHA-256 与报告位置。

### 4.2 表单字段 → `app.Config` 映射

| TUI 字段 | app.Config 路径 | 输入类型 | 校验 |
|----------|-----------------|----------|------|
| 输入 APK | `InputAPK` | 文件选择/输入 | 必须存在，必须是 zip（读文件头） |
| 输出路径 | `FinalOutput` | 输入 | 目录可写 |
| 预设 | （决定下面各节 enabled） | 单选 | — |
| zipalign 路径 | `Zipalign.Path` | 自动探测+输入 | 可执行文件存在 |
| apksigner 路径 | `Signing.ApksignerPath` | 自动探测+输入 | 同上 |
| keystore | `Signing.Keystore` | 文件选择 | 存在；或勾选"自动生成"展开 keytool 参数 |
| keystore 密码 | `Signing.StorePass` | **env 变量名**输入 | 变量已设置；不回显值 |
| key 密码 | `Signing.KeyPass` | 同上（默认 = store） | 同上 |
| key 别名 | `Signing.KeyAlias` | 输入 | 非空 |
| 加密密钥 | `Protections.EncryptionSecret` | **env 变量名**输入 | 变量已设置 |
| 多 DEX/压缩/随机包名 | `Protections.*` | 勾选（预设预填） | — |
| 报告路径 | `Reporting.Path` | 输入 | 目录可写 |

> 密钥类字段**只接受环境变量名**（依赖 P0.3 的 `${VAR}` 展开落地）：TUI 写入 config 的是 `${APK_STORE_PASS}` 这类引用，明文永不落盘、不进 history。

### 4.3 与现有代码的整合点

| 现有机制 | 整合方式 |
|----------|----------|
| `app.LoadConfig`（`internal/app/config.go:99-125`） | TUI 产出完整 config 对象后走同一 `finalize` 校验，不另造配置解析 |
| `app.Tool.Run`（`internal/app/tool.go:72-186`） | S5 直接调用；进度经 `RunReport` 的 steps 结构回传渲染 |
| `applyOverrides`（`cmd/protector/main.go:161-254`） | TUI 产物是"最终态"配置，绕过 override 逻辑；headless `-profile` 路径才走 override |
| 状态记忆 | 新增 `internal/ui/state.go`，仅存路径/别名等**非敏感**字段 |

### 4.4 目录与构建

```
cmd/protector/main.go        # 分发 `ui` 子命令
internal/ui/
├── model.go                 # bubbletea Model：screen 状态机
├── screens/                 # S1–S5 各 screen（Elm 风格独立 Update/View）
├── form.go                  # 字段定义、校验、Config 映射（纯逻辑，可单测）
└── state.go                 # 上次使用记忆（非敏感字段）
```

- `internal/ui/form.go` 不 import bubbletea——纯映射与校验逻辑，保证可表驱动测试；
- `TERM=dumb` / 非 TTY 启动 `ui` 时打印引导改用 `-profile` 并退出码 2；
- 构建不受 `-tags llvm` 影响。

### 4.5 验收标准

1. 新用户不看文档 3 步内完成一次 `full` 预设加固；
2. 生成的 config.json 直接被 `protector -config` 消费结果一致（幂等回灌测试）；
3. 密钥明文不出现在任何 TUI 屏幕、config 产物与 shell history；
4. `internal/ui/form.go` 表驱动测试覆盖全部字段校验分支。

## 5. 实现状态（2026-10）

**已按本设计落地**（详见 [ROADMAP P0](../ROADMAP.md)）：

- `protector ui` 五步向导：`internal/ui`（model.go 状态机、view.go 渲染、form.go 纯逻辑、autodetect.go build-tools 探测、state.go 跨运行记忆）；
- 密钥字段仅收环境变量名；预览/写盘的配置保留 `${VAR}` 引用（依赖已实现的 P0.3 展开）；
- headless `-profile quick|full|sign-only`（`internal/presets`）与非 TTY 退出码 2 降级；
- 子进程输出经 `Tool.Stdout/Stderr` 注入缓冲，失败时展示尾部日志，不再撕裂 TUI 画面。

**与 §4 设计的偏差**：

| 设计 | 实际 | 原因 |
|------|------|------|
| `internal/ui/screens/` 子包 | `internal/ui` 单包，每 screen 独立文件（model.go/view.go） | 共享表单状态，Elm 子模型拆包带来额外管道 |
| state 位于 `~/.config/protector/state.json` | `os.UserConfigDir()/protector/state.json`（macOS 下为 `~/Library/Application Support`） | 遵循平台惯例 |
| S4 的 git work tree 检查 | 实现为**警告级**（可继续执行） | 不少合法工作流就在仓库内出包 |

**尚未实现**：S5 完成后一键复制命令行、配置 diff 视图（当前为整份 JSON 预览）。
