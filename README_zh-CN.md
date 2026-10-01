<h1 align="center">Apk-Protector-Tool</h1>

<p align="center">
  <img src="docs/assets/logo.svg" width="180" alt="Apk-Protector-Tool logo" />
</p>

<p align="center">
  <strong>本地优先的 APK 加固与 LLVM IR 混淆工具链（Go 实现）</strong><br/>
  加固、扫描、对齐、签名全部在你的机器上完成 —— 不上云、不上传。
</p>

<p align="center">
  <a href="https://github.com/Ero-Cat/Apk-Protector-Tool/actions/workflows/ci.yml"><img src="https://github.com/Ero-Cat/Apk-Protector-Tool/actions/workflows/ci.yml/badge.svg" alt="GitHub Actions CI"></a>
  <a href="https://github.com/Ero-Cat/Apk-Protector-Tool/releases"><img src="https://img.shields.io/github/v/release/Ero-Cat/Apk-Protector-Tool?display_name=tag" alt="Release"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=flat&amp;logo=go" alt="Go Version"></a>
  <a href="https://goreportcard.com/report/github.com/Ero-Cat/Apk-Protector-Tool"><img src="https://goreportcard.com/badge/github.com/Ero-Cat/Apk-Protector-Tool" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue" alt="License: MIT"></a>
</p>

<p align="center">
  🇬🇧 <a href="README.md">English docs</a>&nbsp;&nbsp;·&nbsp;&nbsp;🗺
  <a href="docs/ROADMAP.md">开发路线图</a>&nbsp;&nbsp;·&nbsp;&nbsp;🎨
  <a href="docs/design/tui-evaluation.md">TUI 设计</a>&nbsp;&nbsp;·&nbsp;&nbsp;🐛
  <a href="https://github.com/Ero-Cat/Apk-Protector-Tool/issues">反馈问题</a>
</p>

> 💡 觉得有用？欢迎点一个 ⭐，让更多人看到它！

---

## 📋 目录

- [⚠️ 合规声明](#️-合规声明)
- [🚀 快速开始](#-快速开始)
- [🎬 演示](#-演示)
- [✨ 功能特性](#-功能特性)
- [📖 使用手册](#-使用手册)
  - [protector — APK 加固 CLI](#protector--apk-加固-cli)
  - [goprotect — IR 混淆 CLI](#goprotect--ir-混淆-cli)
  - [配置文件](#配置文件)
  - [运行时集成](#运行时集成)
- [🧠 项目理念 —— 为什么再造一个加固工具？](#-项目理念--为什么再造一个加固工具)
- [🗺 开发路线图](#-开发路线图)
- [❓ 常见问题](#-常见问题)
- [🤝 参与贡献](#-参与贡献)
- [📜 版本历史](#-版本历史)
- [📄 许可证](#-许可证)

---

## ⚠️ 合规声明

本工具链仅用于**加固你自有或获得授权的应用** —— 发布工程、防篡改研究与授权范围内的安全评估。使用者需自行确保符合所在地法律与应用平台政策。请不要用它处理他人的 APK。

---

## 🚀 快速开始

### 环境要求

| 工具 | 用途 | 说明 |
|------|------|------|
| Go 1.25+ | 全部功能 | [go.dev/dl](https://go.dev/dl/) |
| Android build-tools | 签名与对齐 | 提供 `zipalign`、`apksigner`、`keytool`（可用 sdkmanager 安装） |
| LLVM（含 C API） | 仅 `goprotect` | 需 `-tags llvm` 编译；其余功能无需 |
| Android NDK | C 运行时 | 仅在把 `runtime/` 链接进 App 时需要 |

### 安装

```bash
# 安装 APK 加固 CLI（任意 Go 1.25+ 环境）
go install github.com/Ero-Cat/Apk-Protector-Tool/cmd/protector@latest
```

或从源码构建：

```bash
git clone https://github.com/Ero-Cat/Apk-Protector-Tool.git
cd Apk-Protector-Tool

# APK 加固 CLI（无外部依赖）
go build -o dist/protector ./cmd/protector

# IR 混淆 CLI（需要 LLVM）
go build -tags llvm -o dist/goprotect ./cmd/goprotect
```

### 第一次运行

```bash
# 1. 对发布 APK 做预检安全扫描（不做任何修改）
protector -input app-release.apk -report dist/report.json

# 2. 完整加固：加密 DEX、对齐、签名、验签
BT=~/Library/Android/sdk/build-tools/36.1.0   # 或 /path/to/sdk/build-tools/XX.X.X

protector \
  -input app-release.apk \
  -output dist/app-protected.apk \
  -protect -protect-multi-dex -protect-compress -protect-random-package \
  -protect-secret "$APK_PROTECT_SECRET" \
  -zipalign "$BT/zipalign" \
  -apksigner "$BT/apksigner" \
  -keystore sign/release.keystore \
  -store-pass "$APK_STORE_PASS" \
  -key-alias "$APK_KEY_ALIAS" \
  -verify \
  -report dist/report.json
```

流水线做的每一件事 —— 扫描发现、保护步骤、产物与 SHA-256 —— 都会写入 `dist/report.json` 供审计。

参数太多？完全可以不碰：

```bash
# 交互向导：选 APK → 选预设 → 填路径（自动探测）→ 执行
export APK_STORE_PASS=...            # 密钥从环境变量读取，绝不手输
protector ui

# 或用预设走 headless
protector -profile full -input app-release.apk \
  -keystore sign/release.keystore -store-pass-env APK_STORE_PASS -key-alias release
```

---

## 🎬 演示

对一个人工埋入 `frida`/`xposed`/`magisk` 字符串和一枚私钥的测试 APK 执行加固的真实输出 —— 流水线推进前，扫描把问题全部揪了出来：

![protector 加固演示](docs/assets/demo-scan.svg)

同一次运行还会产出机器可读报告：

```json
{
  "steps": [
    { "name": "scan",       "status": "completed" },
    { "name": "protections", "status": "completed" },
    { "name": "zipalign",   "status": "completed" }
  ],
  "hashes": { "sha256": "e09eaa0aa3fdf5bd33a3afb6552f79fc..." }
}
```

交互向导（`protector ui`）把同一条流水线变成五步表单 —— build-tools 路径自动探测、密钥按环境变量名引用、生成的配置保留 `${VAR}` 引用而非明文密码：

![protector ui 向导](docs/assets/demo-tui.svg)
<small>S3 配置界面示意图。</small>

---

## ✨ 功能特性

**成熟度标注是诚实的**：✅ = 稳定且有测试覆盖，🧪 = 实验性 / 进行中 —— 具体缺口见[路线图](docs/ROADMAP.md)。

### 🔐 APK 加固流水线 — `protector` ✅

| 功能 | 说明 |
|------|------|
| 交互向导 | `protector ui`：五步 Bubbletea TUI —— build-tools 自动探测、预设选择、密钥走 env 引用、配置预览 |
| Headless 预设 | `-profile quick\|full\|sign-only` 把参数面压缩到输入 + 签名材料 |
| 静态安全扫描 | 检出加固器指纹、内嵌 APK/证书、私钥泄露、反环境关键词（frida、xposed、magisk…）、Janus 签名风险 |
| DEX 加密 | 全部 `classes*.dex` 使用 AES-256-GCM 加密；可选 Deflate 预压缩。v1.6 起密钥**不随 APK 分发**（`<output>.key`，0600）—— 见 [ADR-0001](docs/design/adr-0001-dex-key-delivery.md) |
| 包名随机化 | 等长改写清单包名，干扰静态分析 |
| 伪加固标记 | 内嵌模拟主流商业加固器的特征产物 |
| 第三方加固器钩子 | 以模板化路径/环境变量把外部加固 CLI 包装进流水线 |
| 自动 zipalign | APK 含 native 库时自动启用；Android R+ 强制 `resources.arsc` 不压缩 |
| 签名与验签 | 经 `apksigner` 的 V1+V2 签名、可选 `--print-certs` 验签、keytool 自动生成 keystore |
| JSON 运行报告 | 每个步骤、产物与哈希全部记录，供 CI 审计 |

### 🔀 IR 混淆 — `goprotect` 🧪

| 功能 | 说明 |
|------|------|
| 控制流平坦化 | 以 switch 调度器状态机重写函数 |
| 常量拆分 / 指令替换 | 常量经算术拆分、XOR 等价变换包装 |
| 字面量混淆 | 整数常量改写为运行时可恢复的恒等链；私有字符串全局加密为可写密文、启动时原位还原 —— 明文不再留在产物里 |
| 安全钩子 | 注入反调试与完整性校验的入口/出口调用 |
| `.bc` / `.ll` 输入 | 文本 IR 直接按扩展名识别 |
| CFG 导出 | `-dump-cfg` 前后 DOT 图（graphviz 渲染） |
| 混淆等级 | `low` / `medium` / `high` 预设，调节比率与强度 |

### 🌀 VMP 虚拟化 — 🧪

| 功能 | 说明 |
|------|------|
| 字节码编译器 | 将选定函数从 LLVM IR 编译为自定义 VM 指令集 —— 支持 void 与 i32 返回、最多 4 个 i32 参数 |
| 旧体整体擦除 | 原指令从产物中抹除，只保留入口调用桩 |
| 多 VM 分级 | 拆分 VM（如 `vm_a`/`vm_b`）映射 normal/sensitive/critical 三级函数 |
| Opcode 随机化 | 每次构建生成唯一指令映射，烘焙进字节码数据 |
| 字节码加密 | XOR 加密，密钥拆分为 `静态片段 ^ key_pad` |
| 统一入口 ABI | 单符号 `__goprotect_vm_entry_encrypted(bc, meta, a0..a3) -> i32`，自定义 VM 名不会破坏链接 |

### ⚙️ Android C 运行时 — 🧪

`runtime/` 提供可 NDK 交叉编译的完整组件：VM 解释器（按元数据解码/解密/传参执行）、字符串原位解密、FNV-1a 完整性校验（可配失败策略 LOG/EXIT/ZEROIZE）、反调试（含 Frida 端口探测）。`runtime/android/` 为配套的 NDK demo 加载器：最小 AES-256-GCM + JNI 胶水 + `InMemoryDexClassLoader` 示例，与外置密钥方案（ADR-0001）配对。见[运行时集成](#运行时集成)。

---

## 📖 使用手册

### `protector` — APK 加固 CLI

```
protector run  [options]      完整流水线（配置 + 预设 + flag）
protector scan [options]      仅安全扫描
protector sign [options]      对齐 + 签名（可加 -verify）
protector ui                  交互向导
protector config init         生成带注释的配置模板
protector [options]           旧版扁平模式（已弃用，仍可用）
```

#### 交互向导 — `protector ui`

五步终端向导（Bubbletea）：选择 APK → 选择预设 → 填写路径（build-tools 从 `ANDROID_HOME`/SDK 位置自动探测，取值跨运行记忆）→ 预览生成的配置 → 带阶段进度地执行。

密钥字段只接受**环境变量名**（如 `APK_STORE_PASS`），绝不接受明文 —— 向导会校验变量已 export，写出的配置保留 `${APK_STORE_PASS}` 引用。在无 TTY 的环境（CI、管道）中以退出码 2 退出并提示改用 `-profile`。

#### 预设 — `-profile quick|full|sign-only`

| 预设 | 步骤 | 适用 |
|------|------|------|
| `quick` | 扫描 → 对齐 → 签名 | 发布卫生检查 |
| `full` | 扫描 → 全保护（多 DEX、压缩、包名随机化、伪加固）→ 对齐 → 签名 → 验签 | 最大强度加固 |
| `sign-only` | 对齐 → 签名 → 验签 | 改包后重签 |

预设应用在配置文件之上；显式 flag 仍然优先。配合 env 密钥参数，敏感信息不进 shell history：

```bash
protector -profile full -input app.apk \
  -keystore sign/release.keystore -key-alias release \
  -store-pass-env APK_STORE_PASS -protect-secret-env APK_PROTECT_SECRET
```

优先级从强到弱：**子命令强制**（`scan`/`sign` 决定管线形态）→ **显式 flag**（只有实际传入的 flag 才覆盖配置；`-protect=false` 可关闭）→ **`-profile` 预设** → **配置文件**。注意 `sign` 恒关闭保护，`scan` 无需 Android build-tools。

#### 核心参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-input` | — | 源 APK（必填，或在配置中设置 `input_apk`） |
| `-config` | — | JSON/YAML 配置文件路径 |
| `-profile` | — | 预设基线：`quick`、`full` 或 `sign-only` |
| `-output` | `dist/<名称>-protected.apk` | 最终产物路径 |
| `-report` | — | JSON 运行报告输出路径 |
| `-workdir` | 系统临时目录 | 每次运行临时目录的根 |
| `-keep-workdir` | 关 | 保留中间运行目录便于排查 |
| `-skip-scan` | 关 | 关闭预检安全扫描 |
| `-verify` | 关 | 签名后执行 `apksigner verify --print-certs` |

#### 保护参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-protect` | 关 | 内置保护总开关 |
| `-protect-multi-dex` | 关 | 加密**全部** `classes*.dex` |
| `-protect-dex` | 关 | 仅加密主 `classes.dex` |
| `-protect-compress` | 关 | 加密前先 Deflate 压缩（产物更小） |
| `-protect-random-package` | 关 | 随机化清单包名 |
| `-protect-package-prefix` | `com.protector` | 随机包名前缀 |
| `-protect-secret` | 随机生成 | AES 密钥派生所用密钥 |
| `-protect-secret-env` | — | 持有加密密钥的环境变量名（推荐） |
| `-protect-pseudo` | 关 | 内嵌伪加固产物 |

#### 签名与对齐参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-zipalign` | PATH 上的 `zipalign` | zipalign 路径；设置即启用对齐 |
| `-align-bytes` | 4 | zipalign 对齐字节数 |
| `-apksigner` | PATH 上的 `apksigner` | apksigner 路径；设置即启用签名 |
| `-keystore` | — | V1+V2 签名用 keystore |
| `-store-pass` | — | keystore 密码 |
| `-store-pass-env` | — | 持有 keystore 密码的环境变量名（推荐） |
| `-key-pass` | 同 store-pass | 密钥密码 |
| `-key-pass-env` | — | 持有密钥密码的环境变量名 |
| `-key-alias` | — | 签名密钥别名 |
| `-verify` | 关 | 签名后执行 `apksigner verify --print-certs` |
| `-create-keystore` | 关 | 经 `keytool` 自动生成 keystore |
| `-keytool` | PATH 上的 `keytool` | 自动生成用的 keytool 路径 |
| `-sign-arg` | — | 追加到 apksigner 的原始参数（可重复） |

#### 第三方加固器参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-reinforce-command` | — | 签名前执行的外部加固 CLI |
| `-reinforce-output` | — | 加固器预期输出 APK |
| `-reinforce-arg` | — | 传给加固器的参数（可重复；支持 `{{input_apk}}`、`{{output_apk}}`、`{{work_dir}}`、`{{ts}}` 模板） |
| `-reinforce-env` | — | 加固器的 `KEY=VALUE` 环境变量（可重复） |
| `-reinforce-timeout` | — | 加固器超时，如 `5m` |

### `goprotect` — IR 混淆 CLI

```
goprotect -input module.bc -config config/example.yml -o module_protected.bc
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-input` | — | 输入 LLVM 模块：`.bc` 位码或 `.ll` 文本 IR |
| `-config` | — | JSON/YAML 配置 —— 见 `config/example.yml` |
| `-o` | `obf-<名称>.bc` | 输出位码路径 |
| `-level` | 取配置值 | 覆盖混淆等级：`low` / `medium` / `high` |
| `-dump-cfg` | 关 | Pass 前后导出 DOT 控制流（graphviz 渲染） |

深入文档：[docs/goprotect.md](docs/goprotect.md)。

### 配置文件

两个 CLI 均支持 JSON 或 YAML。配置内的相对路径按配置文件所在目录解析，`~/` 会展开，裸二进制名回退到 `PATH` 查找。

- [`config.json.example`](config.json.example) —— `protector` 完整配置（保护、扫描、第三方加固、对齐、签名含 keystore 自动创建）
- [`config/example.yml`](config/example.yml) —— `goprotect` 完整配置（Pass、等级、多 VM 设置）

优先级：CLI 参数在配置值之上做**启用**覆盖，配置文件是基线（`-profile` 预设位于两者之间）。字符串值支持 `${VAR}` 与 `${VAR:-default}` 环境变量引用 —— 未设置且无默认值的变量会在加载时报错，缺失的密钥快速失败而不是被按字面使用。

### 运行时集成

VMP 保护的函数需要运行时解释器支持，仓库提供 C 骨架：

```c
void __goprotect_check_integrity(uint32_t region_id); // 区域 FNV-1a 校验（goprotect_register_region 注册）
void __goprotect_anti_debug(void);                    // TracerPid/maps/模拟器 + Frida 端口探测
int32_t __goprotect_vm_entry_encrypted(const uint8_t* bc, const char* meta,
                                       int32_t a0, int32_t a1, int32_t a2, int32_t a3); // VM 入口
void __goprotect_decrypt_strings(void);               // 加密字符串全局原位还原
```

```bash
cd runtime && mkdir build && cd build
cmake -DANDROID_ABI=arm64-v8a \
      -DANDROID_NDK=/path/to/ndk \
      -DCMAKE_TOOLCHAIN_FILE=$NDK/build/cmake/android.toolchain.cmake \
      ..
make
```

> 现状（诚实说明）：运行时是真实实现 —— 字节码按随机化元数据解码、解密后带参执行并回传返回值；完整性哈希与 Frida 端口探测已实现且 host 测试覆盖。仍需真机收尾的是：反调试/完整性信号的真机验收，以及把 DEX 加载 demo 接进真实 App（[`runtime/android/README.md`](runtime/android/README.md)）。

---

## 🧠 项目理念 —— 为什么再造一个加固工具？

商业加固器是不回源的黑盒；开源方案通常只覆盖单层。本项目选择了不同的立场：

1. **本地优先，永远。** 你的 APK、keystore 与密钥永不离开本机。没有遥测、没有云端队列、没有厂商锁定。能跑 `go build`，就能跑完整流水线。
2. **配置即代码。** 整条流水线是一个 JSON/YAML 文件，外加记录全部动作的 JSON 报告 —— 可在 PR 里评审、可在 CI 中重放、可事后审计。
3. **两层防护，一个仓库。** `protector` 加固 APK 壳（加密、对齐、签名）；`goprotect` 在代码编译前的 LLVM IR 层做混淆。多数工具只选一层，而攻击者两层都用。
4. **优雅降级。** `llvmwrap` 的 mock 实现让整个仓库在没有 LLVM 的机器上照样构建和测试 —— 贡献者不会被工具链安装卡住。

---

## 🗺 开发路线图

完整计划（逐项状态、代码证据与验收标准）见 [docs/ROADMAP.md](docs/ROADMAP.md)。摘要：

| 阶段 | 主题 | 代表事项 |
|------|------|----------|
| **P0** | 加固 UX 与配置安全 | ✅ TUI 向导、预设、CLI 子命令化、`${VAR}` 展开、env 密钥参数、覆盖语义修复 |
| **P1** | VMP 端到端 | ✅ opcode 映射入元数据、运行时真实解密、分支/icmp/值模型编译，真实 LLVM `lli` 验证 |
| **P2** | Pass 正确性 | ✅ cf-flatten 真实重写、整数与字符串字面量加密、非 void 虚拟化 + 旧体擦除、`.ll` 输入、DOT 导出 |
| **P3** | Android 运行时 | ✅ 真实完整性哈希 + 失败策略、Frida 端口探测、DEX 密钥外置 + NDK 加载器 demo（host 验证） |
| **P4** | 测试基建 | ✅ `CommandRunner` 编排测试、LLVM tag 集成测试（CI 含 lli 语义验证） |

---

## ❓ 常见问题

**必须安装 LLVM 吗？**
不需要。LLVM 只在构建/运行 `goprotect` 时需要（`go build -tags llvm`）。`protector` 的 APK 流水线是纯 Go 加 Android build-tools 二进制。

**怎么避免敲一长串参数？**
运行 `protector ui` 走交互向导，或用 `-profile quick|full|sign-only` 走 headless。密钥经 `-store-pass-env` 类参数或配置里的 `${VAR}` 引用接入，命令行干净且不落 history。

**加密后的 DEX 怎么运行？**
目前开箱即用还跑不起来。流水线把 DEX 加密进 `assets/protector/`，且自 v1.6 起密钥**不再随 APK 分发**（落在产物旁的 `<output>.key`，权限 0600）。恢复路径的 NDK demo 加载器（密钥拆分常量 + AES-256-GCM 解密 + `InMemoryDexClassLoader`）见 [`runtime/android/`](runtime/android/README.md)，含真机验收步骤。把它（或你自己的密钥通道）接进 App 是剩余的集成工作；期间发布卫生仍靠扫描 + 包名随机化 + 签名。

**支持哪些签名方案？**
经 `apksigner` 的 V1 + V2（强制开启 V1/V2）。V3/V4 未实现。

**安装输出 APK 报 `INSTALL_FAILED_INVALID_APK`？**
确认对齐与签名都执行了 —— 常见原因是漏掉 `-zipalign`/`-apksigner`/`-keystore`。

**报 `apksigner: executable file not found`？**
用 `sdkmanager "build-tools;36.1.0"` 安装 build-tools，或通过 `-apksigner`/`-zipalign` 显式传路径。

**输出 APK 太大？**
加 `-protect-compress` 在加密前压缩。

**Android 11+（R）安装报错 −124？**
已修复 —— `resources.arsc` 与 `lib/` 条目强制不压缩存储，符合 Android R 起的要求。

**可以用于生产吗？**
`protector` 流水线稳定且有测试。`goprotect`/VMP/运行时为实验性 —— 依赖前请先看[功能特性](#-功能特性)的成熟度标注与路线图。

---

## 🤝 参与贡献

```bash
go vet ./...          # 静态检查
gofmt -w .            # 格式化（CI 强制）
go test ./...         # 运行测试
```

- 遵循 [Conventional Commits](https://www.conventionalcommits.org/)（`feat:`、`fix:`、`docs:` …），祈使句主题 ≤ 70 字符。
- 测试用标准库 `testing`、表驱动风格；外部二进制一律 mock。
- AI 辅助开发的仓库约定见 [`AGENTS.md`](AGENTS.md) 与 `.agent/`（规则、工作流、技能）。

欢迎在 [github.com/Ero-Cat/Apk-Protector-Tool](https://github.com/Ero-Cat/Apk-Protector-Tool) 提交 issue 与 PR。

---

## 📜 版本历史

| 版本 | 更新内容 |
|------|----------|
| **v1.6** | Pass 正确性轮：整数/字符串字面量加密、非 void 虚拟化 + 旧体擦除、`.ll` 输入、DOT 导出；真实完整性哈希与 Frida 端口探测；DEX 密钥外置 + NDK 加载器 demo；llvm-tag 与编排测试地基 |
| **v1.5** | `protector ui` TUI 向导、CLI 子命令化 + 预设；P0/P1 完成：VMP 在真实 LLVM 上端到端验证（`lli` + C 运行时） |
| **v1.4** | VMP 字节码编译器、控制流平坦化重写、运行时库骨架、单元测试 |
| **v1.3** | 修复 Android R+ 安装问题，`resources.arsc` 不压缩 |
| **v1.2** | 多 VM 随机化 VMP、字节码加密、分级保护 |
| **v1.1** | DEX 压缩加密、体积优化 |
| **v1.0** | 基础加固、签名、验证 |

---

## 📄 许可证

[MIT](LICENSE) © Ero-Cat
