# 开发路线图（ROADMAP）

> 本文档是 Apk-Protector-Tool 的详细开发计划。每个条目包含：**现状**（带代码证据）→ **目标** → **任务拆解** → **验收标准**。
>
> 评估结论基于 2026-10 的全仓代码审查。`protector`（APK 加固流水线）稳定可用；`goprotect`/VMP/运行时半边存在多处"设计已定、实现未通"的停滞点，是本计划的主要对象。

## 总览

| 阶段 | 主题 | 条目数 | 预估工作量 | 状态 |
|------|------|--------|-----------|------|
| [P0](#p0-加固-ux-与配置安全) | 加固 UX 与配置安全（protector） | 5 | M | 🟢 P0.1/P0.3/P0.4 已完成，P0.2 部分完成 |
| [P1](#p1-vmp-端到端打通) | VMP 端到端 | 7 | L | ⏳ 设计完成 |
| [P2](#p2-pass-正确性) | Pass 正确性 | 6 | M–L | ⏳ 部分实现 |
| [P3](#p3-android-运行时完善) | Android 运行时完善 | 3 | M | ⏳ 骨架就绪 |
| [P4](#p4-测试基建) | 测试基建 | 2 | M | ⏳ 策略已定未落地 |

建议顺序：**P0.3 / P0.5（安全修复，小改动大收益）→ P0.1 / P0.2（TUI 与 CLI 重构）→ P1 → P2 → P3 → P4 穿插进行**。

---

## P0 加固 UX 与配置安全

### P0.1 Bubbletea TUI 向导（`protector ui`）✅ 已实现

> 详细交互设计与选型评估见 [docs/design/tui-evaluation.md](design/tui-evaluation.md)。
>
> **落地情况**：五步向导（选 APK → 选预设 → 表单 → 预览 → 执行）已随 `internal/ui` 包交付；build-tools 自动探测、跨运行状态记忆（`os.UserConfigDir()/protector/state.json`）、密钥 env 名输入与校验、`${VAR}` 引用写盘、非 TTY 环境退出码 2 降级均已实现。表单纯逻辑位于 `internal/ui/form.go`（表驱动测试覆盖）。

- **现状**：`protector` 共 28 个扁平 flag（`cmd/protector/main.go:30-63`），README 快速开始示例需要 14 个参数；无预设、无交互模式，运维心智负担重。
- **目标**：`protector ui` 子命令提供表单式向导：选 APK → 选预设 → 路径/密钥 → 预览生成 config → 写盘或直接执行。
- **任务拆解**：
  1. 引入 `charmbracelet/bubbletea` + `bubbles`（文本输入/单选/进度），同步更新 `AGENTS.md` 依赖表与 `.agent/rules/tech_stack_conventions.md`；
  2. 实现 screen 流模型（详见设计文档的 screen 流与字段映射表）；
  3. 密钥字段只接受**环境变量名**，不回显明文；
  4. 生成 config 前展示 diff 预览，确认后写入 `config.json` 或直接调用 `app.Tool.Run`。
- **验收标准**：`protector ui` 在不读文档的情况下 3 步内完成一次完整加固；`TERM=dumb` 或 CI 环境自动降级为报错提示改用 `-profile`。

### P0.2 CLI 重构：子命令 + 预设（`-profile`）🟡 部分完成

> **落地情况**：`-profile quick|full|sign-only` 预设与 `protector ui` 子命令分发已实现（`cmd/protector/main.go` + `internal/presets`）。剩余：`run|scan|sign|config init` 完整子命令化、flag 与配置文件的"独占项"对齐、扁平 flag 兼容期的弃用警告。

- **现状**：单命令扁平 28 flag；配置文件选项与 flag 分裂（`keep_work_dir`、`reinforce.timeout`、`zipalign.alignment`、keystore 生成参数等只能走配置文件）；`-protect-dex` 与 `-protect-multi-dex` 语义重叠。
- **目标**：分组子命令 `protector run|scan|sign|config init` + 内置预设 `-profile quick|full|sign-only`，常见路径压缩到 1–3 个参数。
- **任务拆解**：
  1. 引入轻量子命令路由（标准库 `flag.FlagSet` 即可，避免再引 CLI 框架）；
  2. 定义预设：`quick`（扫描+签名）、`full`（全保护+对齐+签名+验签）、`sign-only`；
  3. 兼容期保留旧扁平 flag（打印 deprecation 警告），两个大版本后移除；
  4. 补齐"配置文件独占项"的 flag 等价物，消除分裂。
- **验收标准**：`protector run -profile full -input app.apk` 等价于现有 14-flag 示例；`protector config init` 生成带注释的 config 模板。

### P0.3 配置 `${VAR}` 环境变量展开 🔴 安全 ✅ 已实现

> **落地情况**：`internal/envref` 包实现 `${VAR}`/`${VAR:-default}`（缺变量报错点名），`internal/app` 与 `config`（goprotect）两侧加载路径均已接入，两侧各有表驱动测试。

- **现状**：`config.json.example:14` 写了 `"encryption_secret": "${APK_PROTECT_SECRET}"`、`config/example.yml:36-37` 写了 `${GOPROTECT_STATIC_KEY}`，但 `internal/app/config.go` 与 `config/config.go` 的加载路径**从不调用** `os.ExpandEnv`——这些字符串会被当作字面密钥使用。示例引导用户踩坑。
- **目标**：配置加载时对字符串字段做 `${VAR}` 展开；变量不存在时报错（除非 `${VAR:-default}`）。
- **任务拆解**：
  1. 在两个 config 包各加 `expandEnv(string) (string, error)`，仅识别 `${…}` 语法，不处理 `$VAR`（避免误伤 Windows 路径等）；
  2. 展开范围：`encryption_secret`、`store_pass`、`key_pass`、`static_key`、命令/路径字段；
  3. 加表驱动测试：展开、缺变量报错、默认值语法、字面 `$` 转义。
- **验收标准**：示例配置 + 环境变量可直接工作；CI 中 `APK_PROTECT_SECRET` 注入后报告里不再出现字面 `${…}`。

### P0.4 env 密钥参数（`-store-pass-env` 等）🔴 安全 ✅ 已实现

> **落地情况**：`-protect-secret-env`/`-store-pass-env`/`-key-pass-env` 三个 flag 已实现，启动时解析并校验变量已设置（未设置给出点名用途的可行动报错）。

- **现状**：`-store-pass`/`-key-pass`/`-protect-secret` 直接传值，会落入 shell history 与 `ps` 进程列表。
- **目标**：新增 `-store-pass-env`/`-key-pass-env`/`-protect-secret-env`，取环境变量名而非值。
- **任务拆解**：main.go 加 3 个 flag；`applyOverrides` 中 env 变体优先于明文变体；文档更新。
- **验收标准**：`protector …-store-pass-env APK_STORE_PASS` 与 `-store-pass` 行为一致；两者同传时 env 优先并打印提示。

### P0.5 覆盖语义与死配置修复 🔴 正确性

- **现状**（三处独立问题）：
  1. `applyOverrides`（`cmd/protector/main.go:161-254`）只能把配置项从关改开，CLI 无法关闭配置中开启的选项（`-skip-scan` 是唯一例外）；
  2. 含 `lib/*.so` 的 APK 自动启用 zipalign（`internal/app/tool.go:261-268`），但二进制缺失时报错晚且信息含糊（tool.go:296），首次演示即踩中；
  3. 死配置项：`Scanning.Keywords`（`internal/app/config.go:210` 默认后从未被 `scanAPK` 读取，扫描用硬编码 `antiEnvKeywords`）；`VMP.EnableMultiVM`（`config/config.go` 从未被读取，多 VM 恒开）。
- **目标**：覆盖语义可预期、错误提前且可行动、无死配置。
- **任务拆解**：
  1. 覆盖语义改为"flag 显式出现即覆盖（含关闭）"——用 `flag.Visit` 判断显式设置，替代"非空/true 才覆盖"；
  2. `finalize` 阶段探测将启用的外部工具（含 zipalign 自动启用场景）存在性，统一报"找不到 X：安装 build-tools 或用 -zipalign 指定路径"；
  3. `Scanning.Keywords` 接入 scanner（空则回退默认表）；`EnableMultiVM=false` 时折叠为单 VM。
- **验收标准**：`-protect=false` 能关闭配置开启的保护；缺 zipalign 时在扫描前即报可行动错误；改 `scanning.keywords` 后扫描结果随之变化。

---

## P1 VMP 端到端打通

> 现状一句话：编译器、字节码格式、C 解释器三件套各有简化与脱节，端到端（Go 编译 → 元数据 → C 解码执行）从未跑通。以下 7 项按依赖顺序排列。

### P1.1 opcode 映射导出到元数据

- **现状**：`passes/vmp/opcodes.go:120-146` 每次构建随机化 opcode 映射，但 `passes/virtualize.go:163-175` 写 `__gp_bc_meta_<fn>` 元数据 JSON 时**漏发 Mapper**——运行时拿不到映射，无法解码任何指令。
- **目标**：元数据包含完整 opcode→byte 映射、字节码长度、VM 名、加密标志。
- **任务拆解**：扩展 meta JSON schema（`opcodes`、`len`、`encrypted`）；`virtualize.go` 序列化 `Mapper`；`runtime/src/vm_entry.c` 解析后建查找表。
- **验收标准**：C 端能按元数据还原与编译期一致的指令解码表（单测：同一 meta JSON 两侧解析结果一致）。

### P1.2 C 解释器补齐缺失 opcode

- **现状**：`opcodes.go` 定义了 `0x46 OP_CMP_ULT`、`0x47 OP_CMP_UGT`、`OP_PUSH_ARG`，`runtime/src/vm_entry.c` 的 dispatch 循环**没有对应 case**（grep 零命中）——执行到即 `unknown opcode` 停机。
- **目标**：解释器覆盖编译器可产出的全部指令。
- **任务拆解**：vm_entry.c 补 3 个 case；建立"opcode 清单单一事实源"（Go 侧生成、C 侧包含的共享头/表）防再脱节。
- **验收标准**：Go 侧遍历 `OpcodeNames` 逐项断言 C 解释器源码含对应处理（构建期一致性测试）。

### P1.3 分支 backpatch 目标修复

- **现状**：`passes/vmp/compiler.go:181,193` 的 backpatch 目标写成合成名 `target_%d`/`true_%d`，与 `DefineLabel` 产生的基本块名不匹配，`LabelResolver.Resolve`（`passes/vmp/bytecode.go:123-125`）静默回退 **offset 0**——所有条件跳转实际跳到字节码开头。
- **目标**：标签解析严格化，跳转目标正确。
- **任务拆解**：编译器统一由 label 表分配名字；`Resolve` 未命中时返回错误而非静默 0；补 `Compiler.Compile` 单测（现在为零覆盖）覆盖含分支的函数。
- **验收标准**：含 if/else 的测试函数编译后，模拟执行路径与原 IR 语义一致（Go 侧字节码解释器单测）。

### P1.4 icmp 谓词完整映射

- **现状**：`compiler.go:157` 所有 icmp 一律编译为 `OP_CMP_EQ`（注释自认"简化处理"）——`<`、`>`、`!=` 语义全部错误。
- **目标**：eq/ne/ult/ugt/slt/sgt 全量映射（配合 P1.2 的解释器 case）。
- **任务拆解**：谓词→opcode 映射表；带符号比较视运行时解释器宽度约定（统一 i32）实现。
- **验收标准**：单测覆盖 6 种谓词的编译输出 opcode 正确。

### P1.5 调用目标符号记录

- **现状**：`compiler.go:211` call 指令的目标名写死 `"unknown"`，`ext_funcs` 元数据因此无意义。
- **目标**：记录真实被调符号，运行时可校验/解析外部调用。
- **任务拆解**：从 llvmwrap call 指令取 callee 名；元数据 `ext_funcs` 去重导出。
- **验收标准**：含 `call @foo` 的模块编译后 meta 中 `ext_funcs` 含 `"foo"`。

### P1.6 运行时解密与真实长度

- **现状**：`runtime/src/vm_entry.c:311-316` 注释明言"此处应解密字节码…简化实现：直接执行"，且 `bytecode_len = 1024; /* 实际应从元数据读取 */`。
- **目标**：按元数据读取长度与密钥提示，解密后再解释执行。
- **任务拆解**：长度/加密标志从 meta 读取（依赖 P1.1）；实现与 Go 侧 `deriveKey` 对齐的解密（AES-GCM 或按 `static_key` 的对称方案，两侧共用测试向量）；解密失败返回错误码而非崩溃。
- **验收标准**：加密字节码经 C 例程解出与编译期一致的明文（共享向量单测）。

### P1.7 补齐未定义钩子符号

- **现状**：`passes/entry_exit.go:9`、`const_obf.go:11` 生成对 `__goprotect_hook`、`__goprotect_decrypt_strings` 的调用，但**全仓无声明无实现**——链接期直接 unresolved symbol。
- **目标**：`runtime/include/goprotect.h` 声明 + `runtime/src/` 提供可链接实现（哪怕先是 no-op 打桩）。
- **任务拆解**：头文件补声明；实现空转版本；CMake 目标验证可独立链接。
- **验收标准**：经 goprotect 处理的 bitcode 与 runtime 一起链接无未解析符号。

---

## P2 Pass 正确性

### P2.1 cf_flatten 终结器分析与重写

- **现状**：`passes/cf_flatten.go:122,135` 注释自认简化——不分析原基本块终结器直接追加 `CreateBr`，对已有终结器的块会**产出非法 IR**；另 `docs/goprotect.md:24` 描述（"简易扰动，后续可扩展为调度器"）与代码现状（已有调度器）不符，文档需同步。
- **目标**：标准平坦化——遍历基本块，剥离原终结器，按后继重写为 `switch state` 分发。
- **任务拆解**：llvmwrap 补终结器读取/删除 API；cf_flatten 重写主循环；pass 结束统一 `LLVMVerifyModule`（联动 P4.2）。
- **验收标准**：对含循环+多分支的 `.ll` 输入，pass 后模块通过 verifier 且语义等价（可用 lli 执行对比）。

### P2.2 const_obf 真实字面量重写

- **现状**：`passes/const_obf.go:11` 注释自认"完整方案应遍历并重写字面量，这里先在入口调用解密存根"——仅插入调用，未动任何常量。
- **目标**：枚举函数内整数常量，替换为运行时解密表达式；配合 P1.7 的解密实现。
- **任务拆解**：llvmwrap 补常量遍历/替换 API；按 `substitute_intensity` 比例抽样改写；保留原值可恢复语义（xor/加法拆分）。
- **验收标准**：改写后函数经 verifier 通过，lli 执行结果与原函数一致。

### P2.3 virtualize 支持非 void 函数

- **现状**：`passes/virtualize.go:59` 注释"Only handle void functions for now"；且旧函数体未移除，只是前面加了个入口块——体积与信息泄露双输。
- **目标**：支持带返回值函数（返回值经 VM 栈回传）；原函数体替换为对 VM 入口的调用桩。
- **任务拆解**：VM 指令集补 `OP_RET_VALUE`；编译器处理 ret；调用桩生成。
- **验收标准**：`virtualize_ratio` 覆盖到非 void 函数，模块 verify 通过。

### P2.4 `--dump-cfg` DOT 导出

- **现状**：flag 解析了（`cmd/goprotect/main.go:40`）、配置字段存在（`config/config.go:44`）、文档标注"预留开关，目前存根"（`docs/goprotect.md:18`）——**没有任何消费代码**。
- **目标**：pass 前后各导出 `dump-cfg-before.dot` / `dump-cfg-after.dot`。
- **任务拆解**：llvmwrap 补基本块/后继遍历 API；DOT 生成器（纯字符串拼接即可）；`debug.dump_cfg` 接线。
- **验收标准**：`graphviz` 渲染输出无报错，before/after 差异肉眼可见（平坦化效果可视化）。

### P2.5 `.ll` 文本输入支持

- **现状**：`-input` 帮助文案宣称接受 `.bc`/`.ll`，但 `llvmwrap/native.go` 只有 `LLVMParseBitcode2`——`.ll` 实际不可用。
- **目标**：按扩展名分发，`.ll` 走 `LLVMParseIRInContext`。
- **任务拆解**：native.go 补文本解析；goprotect 入口按扩展名选择。
- **验收标准**：对同一模块的 `.bc` 与 `.ll` 两种输入，pass 输出等价。

### P2.6 llvmwrap 空实现补齐

- **现状**：`llvmwrap/native.go:128-134` 的 `appendInstructionBefore`（空函数体）与 `terminateWith`（noop 占位）两个方法无实际行为——是 P2.1/P2.2 的前置依赖。
- **目标**：两个方法对接真实 LLVM C API。
- **任务拆解**：实现 + 在 `-tags llvm` 构建下冒烟验证。
- **验收标准**：native 构建下调用两方法后模块 verify 通过。

---

## P3 Android 运行时完善

### P3.1 完整性校验真实化

- **现状**：`runtime/src/integrity.c:29,79`——期望哈希标注"实际应用中应从安全存储加载"，校验循环"实际实现应读取对应内存区域并计算哈希"，实际直接 `verified = 1`。
- **目标**：对指定内存区域计算哈希并与期望值比对，不匹配走预设响应（退出/擦除）。
- **任务拆解**：区域→范围表（构建期生成）；哈希算法与期望值存储格式定义；失败策略可配置。
- **验收标准**：篡改受保护区域后校验返回失败（NDK 交叉编译单测）。

### P3.2 Frida 端口检测

- **现状**：`runtime/src/antidebug.c:85` 注释"省略实现：需要网络 socket 检测"。
- **目标**：检测本机 27042/27043 等 frida-server 默认端口与 `frida-gadget` 加载痕迹。
- **任务拆解**：socket 连接探测 + `/proc/self/maps` 扫描；结果并入安全钩子上报通道。
- **验收标准**：设备上运行 frida-server 时检测函数返回阳性（真机验收）。

### P3.3 DEX 加载/解密运行时 + 密钥存储重设计 🔴 安全

- **现状**：`internal/app/protections.go:96,451` 把加密密钥 base64 写进 APK 内的 `assets/protector/metadata.json`——**密钥与密文同体**，对抗静态提取无意义；且仓库不含任何 Android 侧 DEX 加载/解密组件，加密后的包开箱跑不起来。
- **目标**：提供 NDK 示例加载器（native 拿密钥→解密→`InMemoryDexClassLoader`），密钥不再随包明文分发。
- **任务拆解**：
  1. 方案决策：密钥改由 NDK 侧编译期注入 + 白盒/混淆，或 JNI 拼装 + 证书绑定（先出 ADR 记录取舍）；
  2. 实现最小加载器 demo（Application 替换或 attach hook）；
  3. protector 侧配合：`metadata.json` 拆分为"运行时必需元数据"与"构建期信息"两份。
- **验收标准**：一个 demo App 经 protector 加密后能在真机启动并正常运行；APK 内不再含可直接使用的密钥材料。

---

## P4 测试基建

### P4.1 CommandRunner 抽象落地

- **现状**：`.agent/workflows/testing_strategy.md` 规定"外部二进制经 `CommandRunner` 接口 mock"，但 `internal/app/tool.go` 直接 `exec.Command` 调 zipalign/apksigner/keytool——策略停留在文档，签名/对齐路径零测试。
- **目标**：外部调用全部经接口注入，CI 无 Android 工具也能测流水线编排逻辑。
- **任务拆解**：定义 `CommandRunner` 接口；tool.go/scanner.go 改造注入；表驱动测试覆盖"工具缺失/失败/成功"三类路径。
- **验收标准**：`internal/app` 对 scan→protect→align→sign→report 的编排逻辑获得无外部依赖的测试覆盖。

### P4.2 LLVM 集成测试（`-tags llvm`）

- **现状**：所有 pass 测试只断言**名称与管线拼装**，从未对真实 IR 验证变换正确性；`llvmwrap/mock.go` 使默认构建下一切返回 `ErrNoLLVM`。
- **目标**：带 build tag 的集成测试：真实解析 `.ll` → 跑 pass → `LLVMVerifyModule` → 断言结构。
- **任务拆解**：`passes/*_llvm_test.go`（`//go:build llvm`）；固定小规模 `.ll` 夹具；CI 增加带 LLVM 的可选 job（apt 装 llvm-dev，允许失败标记过渡）。
- **验收标准**：本地 `go test -tags llvm ./passes/...` 通过，并实际暴露/防住 P2 类正确性回归。

---

## 风险与外部依赖

| 依赖 | 影响 | 缓解 |
|------|------|------|
| Charm 栈（bubbletea/bubbles） | P0.1 引入直接+间接依赖，动摇"仅 yaml.v3"现状 | 锁定次版本；AGENTS.md 与规则文档同步更新 |
| LLVM 版本矩阵 | P1/P2 的 C API 行为随版本漂移 | 明确支持版本（如 LLVM 15–18）；pkg-config 探测进 CI |
| Android NDK / 真机 | P3 验收需要设备 | 模拟器兜底 + 篡改类用例降级为 host 编译单测 |
| goprotect 与 runtime 的 schema 漂移 | 元数据/opcode 两侧脱节（已发生，见 P1.1/P1.2） | 单一事实源 + 构建期一致性测试 |

## 维护性附注

- `docs/goprotect.md` 与代码现状有多处不同步（如 cf_flatten 描述停留在"简易扰动"），P2 各条目落地时同步修订。
- `AGENTS.md` 提及的 `sign/` 目录在仓库中不存在（本地 gitignore 材料），README 中已按"本地自备"表述。
