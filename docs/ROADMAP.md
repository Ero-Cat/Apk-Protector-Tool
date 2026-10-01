# 开发路线图（ROADMAP）

> 本文档是 Apk-Protector-Tool 的详细开发计划。每个条目包含：**现状**（带代码证据）→ **目标** → **任务拆解** → **验收标准**。
>
> 评估结论基于 2026-10 的全仓代码审查。`protector`（APK 加固流水线）稳定可用；`goprotect`/VMP/运行时半边存在多处"设计已定、实现未通"的停滞点，是本计划的主要对象。

## 总览

| 阶段 | 主题 | 条目数 | 预估工作量 | 状态 |
|------|------|--------|-----------|------|
| [P0](#p0-加固-ux-与配置安全) | 加固 UX 与配置安全（protector） | 5 | M | ✅ 全部完成 |
| [P1](#p1-vmp-端到端打通) | VMP 端到端 | 8 | L | ✅ 全部完成；真实 LLVM 链路已验证（lli 语义正确 + 真实 C 运行时执行） |
| [P2](#p2-pass-正确性) | Pass 正确性 | 6 主条目 / 20 子项 | M–L | ✅ 全部完成（P2.1–P2.6，真实 LLVM 链路 lli 语义验证） |
| [P3](#p3-android-运行时完善) | Android 运行时完善 | 3 主条目 / 11 子项 | M–L | ⏳ 已细颗粒拆分；host 可验证部分推进中 |
| [P4](#p4-测试基建) | 测试基建 | 2 主条目 / 7 子项 | M | ⏳ 已细颗粒拆分 |

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

### P0.2 CLI 重构：子命令 + 预设（`-profile`）✅ 已实现

> **落地情况**：完整子命令化已交付——`protector run|scan|sign|ui|config init`。scan 强制仅扫描、sign 强制仅对齐+签名；旧扁平模式完全兼容并打印弃用警告；补齐了配置独占项的 flag 等价物（`-keep-workdir`、`-reinforce-timeout`、`-align-bytes`）；`config init` 生成带注释 YAML 模板（密钥一律 `${VAR}` 引用）。

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

### P0.5 覆盖语义与死配置修复 🔴 正确性 ✅ 已实现

> **落地情况**：
> 1. 覆盖语义——`flag.Visit` 识别显式传入的参数，显式 `-protect=false` 可关闭配置启用的选项，并连带清掉非显式子开关（防止 finalize 隐式复活）；
> 2. 预检——`Tool.preflight` 在管线启动前校验 apksigner/keytool/zipalign（含隐式要求），给出可行动报错；语义细化：**纯扫描等不改写产物的运行不再隐式要求 zipalign**（对齐仅当保护/加固/签名会改写产物时自动启用）；
> 3. 死配置——`scanning.keywords` 已接入 scanner（空则回退内置表，有测试锚定）；`vmp.enable_multi_vm=false` 折叠为单 VM 并重映射等级。

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

### P1.1 opcode 映射导出到元数据 ✅ 已实现

> **落地情况**：`bytecodeMetadata` 新增 `opcodes`（标准助记符→随机化字节）、`encrypted`、`key_pad`、`param_count` 字段，`metadataJSON` 序列化 `CompileResult.Mapper`；形状测试锚定（`passes/virtualize_meta_test.go`）。

- **现状**：`passes/vmp/opcodes.go:120-146` 每次构建随机化 opcode 映射，但 `passes/virtualize.go:163-175` 写 `__gp_bc_meta_<fn>` 元数据 JSON 时**漏发 Mapper**——运行时拿不到映射，无法解码任何指令。
- **目标**：元数据包含完整 opcode→byte 映射、字节码长度、VM 名、加密标志。
- **任务拆解**：扩展 meta JSON schema（`opcodes`、`len`、`encrypted`）；`virtualize.go` 序列化 `Mapper`；`runtime/src/vm_entry.c` 解析后建查找表。
- **验收标准**：C 端能按元数据还原与编译期一致的指令解码表（单测：同一 meta JSON 两侧解析结果一致）。

### P1.2 C 解释器补齐缺失 opcode ✅ 已实现

> **落地情况**：解释器改为经 **decode[256] 表**把随机化字节还原为标准 opcode 后再 dispatch（此前静态 switch 根本无法解码任何随机化指令）；补齐 `OP_CMP_ULT/UGT/ULE/UGE`（ISA 同步扩展 0x48/0x49）；Go/C 两侧 opcode 表一致性由 `passes/vmp/opcodes_consistency_test.go` 构建期校验（值+助记符+switch 覆盖三重断言）。

- **现状**：`opcodes.go` 定义了 `0x46 OP_CMP_ULT`、`0x47 OP_CMP_UGT`、`OP_PUSH_ARG`，`runtime/src/vm_entry.c` 的 dispatch 循环**没有对应 case**（grep 零命中）——执行到即 `unknown opcode` 停机。
- **目标**：解释器覆盖编译器可产出的全部指令。
- **任务拆解**：vm_entry.c 补 3 个 case；建立"opcode 清单单一事实源"（Go 侧生成、C 侧包含的共享头/表）防再脱节。
- **验收标准**：Go 侧遍历 `OpcodeNames` 逐项断言 C 解释器源码含对应处理（构建期一致性测试）。

### P1.3 分支 backpatch 目标修复 ✅ 已实现

> **落地情况**：编译器第一遍建立 `RefID→稳定标签` 表（匿名块按位置命名），br 的真实/假分支均回填**真实块标签**（条件跳转展开为 JNZ+JMP 双回填）；`LabelResolver.Resolve` 对未知标签报错而非静默落 offset 0（测试锚定）。

- **现状**：`passes/vmp/compiler.go:181,193` 的 backpatch 目标写成合成名 `target_%d`/`true_%d`，与 `DefineLabel` 产生的基本块名不匹配，`LabelResolver.Resolve`（`passes/vmp/bytecode.go:123-125`）静默回退 **offset 0**——所有条件跳转实际跳到字节码开头。
- **目标**：标签解析严格化，跳转目标正确。
- **任务拆解**：编译器统一由 label 表分配名字；`Resolve` 未命中时返回错误而非静默 0；补 `Compiler.Compile` 单测（现在为零覆盖）覆盖含分支的函数。
- **验收标准**：含 if/else 的测试函数编译后，模拟执行路径与原 IR 语义一致（Go 侧字节码解释器单测）。

### P1.4 icmp 谓词完整映射 ✅ 已实现

> **落地情况**：llvmwrap 新增 `Instruction.ICmpPredicate()`（对接 `LLVMGetICmpPredicate`，native+mock），编译器按全部 10 个 LLVM 整数谓词映射 VM 指令（无符号 ULE/UGE 为本次 ISA 扩展）；表驱动测试覆盖。

- **现状**：`compiler.go:157` 所有 icmp 一律编译为 `OP_CMP_EQ`（注释自认"简化处理"）——`<`、`>`、`!=` 语义全部错误。
- **目标**：eq/ne/ult/ugt/slt/sgt 全量映射（配合 P1.2 的解释器 case）。
- **任务拆解**：谓词→opcode 映射表；带符号比较视运行时解释器宽度约定（统一 i32）实现。
- **验收标准**：单测覆盖 6 种谓词的编译输出 opcode 正确。

### P1.5 调用目标符号记录 ✅ 已实现

> **落地情况**：llvmwrap 新增 `Instruction.CalledValue()`（对接 `LLVMGetCalledValue`），call 编译取真实被调符号（回退 operand 0 名称），参数压栈跳过被调函数值；`ext_funcs` 元数据因此有真实内容。

- **现状**：`compiler.go:211` call 指令的目标名写死 `"unknown"`，`ext_funcs` 元数据因此无意义。
- **目标**：记录真实被调符号，运行时可校验/解析外部调用。
- **任务拆解**：从 llvmwrap call 指令取 callee 名；元数据 `ext_funcs` 去重导出。
- **验收标准**：含 `call @foo` 的模块编译后 meta 中 `ext_funcs` 含 `"foo"`。

### P1.6 运行时解密与真实长度 ✅ 已实现

> **落地情况**：VM 入口改为 `(bytecode, meta)` 双参数（Go 侧跳板同步传元数据全局）；运行时从元数据读取 `bytecode_len`/`encrypted`/`key_pad`，解密采用密钥拆分模型——`key = 注册静态片段 ^ key_pad`，静态片段经 `goprotect_set_static_key` 注入（默认 0x5A 与编译期一致），**不随 APK 元数据分发**；解密在独立堆缓冲进行，失败拒绝执行。

- **现状**：`runtime/src/vm_entry.c:311-316` 注释明言"此处应解密字节码…简化实现：直接执行"，且 `bytecode_len = 1024; /* 实际应从元数据读取 */`。
- **目标**：按元数据读取长度与密钥提示，解密后再解释执行。
- **任务拆解**：长度/加密标志从 meta 读取（依赖 P1.1）；实现与 Go 侧 `deriveKey` 对齐的解密（AES-GCM 或按 `static_key` 的对称方案，两侧共用测试向量）；解密失败返回错误码而非崩溃。
- **验收标准**：加密字节码经 C 例程解出与编译期一致的明文（共享向量单测）。

### P1.7 补齐未定义钩子符号 ✅ 已实现

> **落地情况**：新增 `runtime/src/hooks.c` 提供 `__goprotect_hook` 与 `__goprotect_decrypt_strings` 的可链接实现（含 `goprotect_set_hook_callback` 事件回调；解密存根 no-op 待 P2.2 字面量改写后填充）；goprotect.h 同步全部声明；CMake 加入构建；四个 C 源文件 clang 语法检查通过。

- **现状**：`passes/entry_exit.go:9`、`const_obf.go:11` 生成对 `__goprotect_hook`、`__goprotect_decrypt_strings` 的调用，但**全仓无声明无实现**——链接期直接 unresolved symbol。
- **目标**：`runtime/include/goprotect.h` 声明 + `runtime/src/` 提供可链接实现（哪怕先是 no-op 打桩）。
- **任务拆解**：头文件补声明；实现空转版本；CMake 目标验证可独立链接。
- **验收标准**：经 goprotect 处理的 bitcode 与 runtime 一起链接无未解析符号。

---

### P1.8 编译器值模型（def-use 槽位绑定）✅ 已实现

- **现状**：原 `compileValue` 给每个非立即数操作数分配全新局部槽，运算结果只留栈上、从不写回——`PUSH_LOCAL` 读到的槽恒为零值，虚拟化函数除纯常量表达式外计算结果错误。
- **目标**：SSA 值 ↔ 局部槽一一绑定，操作数按定义指令解析，结果写回；不支持的指令（访存/调用/phi/参数引用）保守跳过整个函数。
- **任务拆解**：ISA 新增 `OP_STORE_LOCAL(0x32)`；编译器预扫描建立 `指令RefID→槽` 表；操作数解析（常量/定义槽，其余报错）；二元/比较/转型指令结果 `STORE_LOCAL` 写回；默认分支从 NOP 改为报错。
- **验收标准**：C 解释器单测覆盖 槽位往返/算术/安全拒绝/明文回退（`runtime/tests/test_vm.c`）；lli 端到端语义正确。✅ 全部达成（2026-10：`mark` 函数经完整管线虚拟化后由真实 C 运行时解码执行，主程序语义不变）。

## P2 Pass 正确性

### P2.1 cf_flatten 终结器分析与重写 ✅ 已实现

> **落地情况**：真实重写完成——预扫描 phi 检测（含 phi 函数跳过）、终结器分析与擦除（`LLVMInstructionEraseFromParent`）、边改写为"写状态+跳调度器"、条件边拆双跳板、entry 不入 switch case。逐 pass verifier + lli 语义验证（100% 平坦化下 `add(3,4)+loop(4)=13` 正确）。

- **现状**：`passes/cf_flatten.go:122,135` 注释自认简化——不分析原基本块终结器直接追加 `CreateBr`，对已有终结器的块会**产出非法 IR**；另 `docs/goprotect.md:24` 描述（"简易扰动，后续可扩展为调度器"）与代码现状（已有调度器）不符，文档需同步。
- **目标**：标准平坦化——遍历基本块，剥离原终结器，按后继重写为 `switch state` 分发。
- **任务拆解**：llvmwrap 补终结器读取/删除 API；cf_flatten 重写主循环；pass 结束统一 `LLVMVerifyModule`（联动 P4.2）。
- **验收标准**：对含循环+多分支的 `.ll` 输入，pass 后模块通过 verifier 且语义等价（可用 lli 执行对比）。

### P2.2 const_obf 真实字面量重写 ✅ 已实现

> **落地情况**（2026-10）：
> 1. **整数常量全量改写**——遍历全部指令的常量整型操作数，按 `substitute_intensity` 概率替换为 `(x^k)^k` 或 `(x−k)+k` 恒等表达式；终结器与 phi 跳过（switch case 值必须常量 / phi 入参支配性）。**关键修复**：IRBuilder 对全常量操作数做常数折叠，直接 `xor(c,k)` 会折回常量静默消失——密钥改为经 alloca+store+load 栈槽中转（load 值非常量，链得以保留），该修复同时救治了 instr_sub 的同源潜伏缺陷。
> 2. **字符串全局加密**——私有 const i8 数组全局（使用者全为指令）替换为可写密文全局并删除原全局；导出 `__gp_str_regions`/`__gp_str_regions_count` 区域表（{ptr,i32,i8}，与 C 侧结构布局一致）。
> 3. **C 侧真实现**——`__goprotect_decrypt_strings` 一次性守卫 + 表驱动原位 XOR；表与调用同 pass 产出，符号总是成对解析。
> 4. **测试锚定**——llvm-tagged：强度 10 时字面量消失、强度 0 零改写、明文不再出现在 IR、lli 输出原文（真实运行时解密）；`runtime/tests/test_hooks.c`：解密往返/幂等/空槽跳过。

- **P2.2.1 llvmwrap 常量与使用者 API** ✅（Globals/Users/IsInstruction/IsPrivateLinkage/IsGlobalConstant/Initializer/ConstantDataArrayBytes/AddGlobalBytes/EmitStrRegionsTable/DeleteGlobal，native+mock）
- **P2.2.2 整数常量按强度全量改写** ✅（xor 恒等 + 加法拆分双形态；`passes/wrap.go` 共用）
- **P2.2.3 私有字符串全局加密** ✅（常量使用者整组跳过；`__gp_*`/`__goprotect_*` 前缀排除）
- **P2.2.4 C 侧解密真实现** ✅（hooks.c + goprotect.h 区域结构）
- **P2.2.5 测试锚定** ✅（两侧 CI 绿）

### P2.3 virtualize 完整虚拟化（非 void + 参数 + 旧体擦除）✅ 已实现

> **落地情况**（2026-10）：
> 1. **ISA**——`OP_RET_VALUE(0x4A)` 三处同步（Go opcodes / C 表+case / 一致性测试）；
> 2. **llvmwrap**——`Function.ParamCount()/Param(i)`、`Builder.CreateRet(v)`、`BasicBlock.Delete()`；
> 3. **编译器参数模型**——参数占据 locals[0..n)，编译器槽位从 n 起分配；`PUSH_ARG` 解析参数引用；`param_count` 元数据真实化；新增 `MaxLocals=64` 编译期上限（防 C 侧数组越界 UB）；
> 4. **ret 带值**——`ret v` → 压栈 + `RET_VALUE`；资格收紧：返回 void/i32、参数全 i32 且 ≤4（i64 截断风险保守跳过）；
> 5. **ABI v2**——统一符号 `__goprotect_vm_entry_encrypted(i8*, i8*, i32×4) → i32`（vm_a/vm_b 保留同签名别名），自定义 VM 名不再有链接陷阱；C 侧入口把实参播种进 locals、回传 retval；
> 6. **旧体擦除**——逆序（块逆序×指令逆序，phi 已排除保证支配序）抹掉全部指令再删空块，原指令不再残留在产物；
> 7. **测试**——test_vm.c 增 retval/args 用例；llvm-tagged 断言非 void 虚拟化后 IR 无原指令、lli 经真实 C 运行时语义正确（add(3,4) 由 VM 执行）。
>
> **过程中发现并修复的两个潜伏缺陷**：
> - **密钥错配**——默认 `static_key: c0ffee42` 末字节 0x32 ≠ C 侧默认 0x5A，默认配置产出的字节码永远解不开；现默认留空（两侧统一 0x5A），显式配置时应用须 `goprotect_set_static_key` 注册同值；
> - **pass 顺序缺陷**——virtualize 原排在 entry_exit 之后，而钩子调用会让函数不可编译：默认配置下 virtualize **从未**生效过。现 virtualize 排管线第一位（编译原始函数体，插桩类 pass 在其后处理剩余原生函数）。

### P2.4 `--dump-cfg` DOT 导出 ✅ 已实现

> **落地情况**：llvmwrap 新增 `BasicBlock.Successors()`（经终结器 Value 调 LLVMGetNumSuccessors/GetSuccessor，对 br/condbr/switch 通用）；`passes/dot.go` 生成 per-function cluster、入口高亮、终结器 opcode 标注的 DOT；`pipeline.Run` 在 `debug.dump_cfg` 时把 `dump-cfg-before/after.dot` 写到输出目录；CI llvm job 用 graphviz `dot -Tsvg` 双文件渲染校验。

- **P2.4.1 Successors API** ✅
- **P2.4.2 DOT 生成器** ✅（`passes/dot.go`，另有 mock 兼容单测）
- **P2.4.3 管线接线** ✅（`pipeline.Run` 前后写文件，输出目录跟随 cfg.Output）
- **P2.4.4 渲染验证** ✅（CI graphviz 渲染断言 + llvm-tagged 结构断言）

### P2.5 `.ll` 文本输入支持 ✅ 已实现

> **落地情况**：`parseBitcodeImpl` 按扩展名分发——`.ll` 走 `LLVMParseIRInContext`（接管内存缓冲），`.bc` 走 `LLVMParseBitcode2`；llvm-tagged 测试断言同一模块两种输入渲染一致（归一化 ModuleID 头）；CLI 默认输出统一为 `.bc` 后缀。

- **P2.5.1 文本解析** ✅
- **P2.5.2 等价性验证** ✅（`TestTextualAndBitcodeInputsEquivalent`）
- **P2.5.3 文档同步** ⏳（随 P2 全部完成后统一修订 goprotect.md）

### P2.6 llvmwrap 空实现补齐 ✅ 已完成（审计轮）

> **落地情况**：原 `appendInstructionBefore`/`terminateWith` 两个死桩已在审计修复轮删除（零调用方）；P2.1 的真实 cf_flatten 重写用 Builder API 直接构建，不再需要这两个方法。本条目关闭。

---

## P3 Android 运行时完善

### P3.1 完整性校验真实化 ⏳

- **现状**：`runtime/src/integrity.c` 注册时直接 `verified = 1`，从不计算哈希；`g_integrity_failures` 永不增长；全仓无任何 `goprotect_register_region` 调用（pass 产出的检查 id 全部落入 "unknown region" 告警）。
- **P3.1.1 真哈希与基线复检** ⏳：实现 FNV-1a64；注册时对 `[start, start+len)` 算基线，检查时复检比对。
- **P3.1.2 失败策略** ⏳：不匹配累计 `g_integrity_failures` 并按策略执行（LOG / EXIT / ZEROIZE，`goprotect_set_integrity_policy` 可配）；未注册 id 一次性告警放行，防 pass 产出 id 误报。
- **P3.1.3 host 单测** ⏳：`runtime/tests/test_integrity.c`（基线通过 / 篡改检出 / 未知区域放行），接入 CI runtime job 与 e2e。
- **验收标准**：篡改已注册区域后校验返回失败且计数增长（host 可验证；NDK 交叉编译矩阵列为后续）。

### P3.2 Frida 端口检测 ⏳

- **现状**：`runtime/src/antidebug.c` 的 check_frida 已有 `/proc/self/maps` 扫描，端口探测注释"省略实现：需要网络 socket 检测"。
- **P3.2.1 端口探测函数** ⏳：跨平台 `goprotect_probe_port()`（非阻塞 connect + 150ms poll/select）。
- **P3.2.2 并入 check_frida** ⏳：探测 27042/27043 并入现有检测结果通道。
- **P3.2.3 host 单测** ⏳：`runtime/tests/test_antidebug.c`（临时监听端口→阳性；关闭后→阴性）。
- **验收标准**：host 单测双向验证；真机 frida-server 阳性为文档化手动验收步骤。

### P3.3 DEX 密钥外置 + NDK 加载器 demo 🔴 安全 ⏳

- **现状**：`internal/app/protections.go` 把 AES 密钥 base64 写进 APK 内 `assets/protector/metadata.json`——密钥与密文同体，对抗静态提取无意义；仓库不含 Android 侧加载/解密组件，加固包开箱跑不起来。
- **P3.3.0 ADR** ⏳：`docs/design/adr-0001-dex-key-delivery.md` 记录取舍（编译期注入拆分 vs JNI 拼装+签名绑定 vs 白盒密码）。
- **P3.3.1 metadata 拆分与密钥外置** ⏳：in-APK 元数据只留非机密（算法/nonce/长度/映射）；密钥写 `<final_output>.key`（0600）；保留 legacy 内嵌逃生开关（默认关，文档标注不安全）。
- **P3.3.2 NDK 加载器 demo** ⏳：`runtime/android/`——mini AES-GCM C 实现（与 Go crypto 向量双锚定）、JNI 解密交付、Java `InMemoryDexClassLoader` 示例；解密核心保持 host 可编译可测。
- **P3.3.3 host 等价验收** ⏳：e2e 断言加固产物无密钥材料（原文/base64/hex 三态扫描）；C 侧共享 golden 向量解密 Go 密文还原原始 dex 字节。
- **P3.3.4 真机手动验收** ⏳：demo App 真机启动步骤文档化。
- **验收标准**：APK 内不再含可直接使用的密钥材料（host 可验证）；真机启动为手动步骤。

---

## P4 测试基建

### P4.1 CommandRunner 抽象落地 ⏳

- **现状**：`.agent/workflows/testing_strategy.md` 规定"外部二进制经 `CommandRunner` 接口 mock"，但 `internal/app/tool.go` 直接 `exec.Command` 调 zipalign/apksigner/keytool——策略停留在文档，签名/对齐编排路径零测试。
- **P4.1.1 接口抽象** ⏳：`CommandRunner` 接口（运行 + LookPath），真实实现为默认值，Tool/Scanner 注入。
- **P4.1.2 编排测试** ⏳：表驱动覆盖"工具缺失/命令失败/成功"三路径 × scan→protect→align→sign→verify→report 的错误信息与报告步骤。
- **P4.1.3 e2e 保持绿** ⏳：真实二进制路径行为不变。
- **验收标准**：`internal/app` 编排逻辑获得无外部依赖的测试覆盖。

### P4.2 LLVM 集成测试（`-tags llvm`）✅ 已实现

> **落地情况**：`passes/testdata/` 四个固定夹具（arith/branchy/strings/calltarget，含 void、非 void、phi 循环、switch、字符串全局）；`passes/pipeline_llvm_test.go` + `llvmwrap/native_llvm_test.go` 覆盖：解析/写回/渲染往返、`.bc`/`.ll` 等价（归一化 ModuleID）、逐 pass 结构断言（cf_flatten 调度器 switch）、lli 前后语义等价（exit code 13/20，runtime.o 经 `GOPROTECT_TEST_RUNTIME_OBJ` 注入，缺失即 Skip）；CI llvm job 增装 llvm-18-runtime + graphviz、编译 runtime.o、跑 `go test -tags llvm ./llvmwrap/... ./passes/...` 并校验 DOT 渲染（job 保持 continue-on-error 过渡）。

- **P4.2.1 夹具** ✅
- **P4.2.2 llvm-tagged 测试** ✅
- **P4.2.3 CI 接入** ✅
- **P4.2.4 llvmwrap 冒烟** ✅
- **验收标准** ✅：本地 `go test -tags llvm ./passes/... ./llvmwrap/...` 通过（LLVM 23 验证）；后续 P2.2/P2.3 的回归断言直接挂在该地基上。

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
