# goprotect 编译期保护工具

本文介绍如何在 Android NDK/LLVM 构建流水线中使用 `goprotect` 对本地库的 LLVM IR/bitcode 进行编译期混淆与安全加固。

## 快速使用

```bash
# 假设已安装匹配版本的 LLVM（含 pkg-config 的 llvm.pc），并开启 cgo：
go build -tags llvm ./cmd/goprotect

# 输入支持 .bc 位码与 .ll 文本 IR（按扩展名分发）
./goprotect --config config/example.yml --input input.ll -o output.bc

# 平坦化效果可视化：pass 前后各导出一份 DOT（graphviz 渲染）
./goprotect --input input.ll --dump-cfg -o output.bc
dot -Tsvg dump-cfg-after.dot -o after.svg
```

- `--config`：YAML/JSON 配置，控制启用的 Pass、函数白/黑名单及混淆强度。
- `--input` / `-o`：输入（`.bc`/`.ll`）与输出（`.bc`）路径。
- `--level`：临时覆盖 `obfuscation.level`（low/medium/high）。
- `--dump-cfg`：把 pass 前后的 CFG 导出为 `dump-cfg-before.dot` / `dump-cfg-after.dot`（写到输出目录）。

## Pass 一览与执行顺序

| 顺序 | Pass | 开关 | 行为 |
|------|------|------|------|
| 1 | virtualize | `passes.virtualization` | **最先执行**：编译原始函数体为 VM 字节码，原体整体擦除，替换为统一入口调用桩 |
| 2 | entry-exit | `passes.entry_exit` | 入口/每个 ret 前调用 `__goprotect_hook` |
| 3 | const-split | `passes.const_split` | 常量拆分改写 add |
| 4 | instr-sub | `passes.instr_substitute` | 按 `substitute_intensity` 概率用恒等链包裹整型操作数 |
| 5 | cf-flatten | `passes.cf_flatten` | 调度器式控制流平坦化（switch 分发循环） |
| 6 | const-obf | `passes.const_obfuscation` | 整数常量恒等改写 + 私有字符串全局加密 |
| 7 | security-hooks | `passes.security_hooks` | 入口调用 `__goprotect_check_integrity` / `__goprotect_anti_debug` |

> virtualize 排在第一位是刻意设计：任何插桩/包裹都会让函数体无法编译为字节码
> （含 alloca/调用/phi），先虚拟化才能保住"原始函数体"这份输入。每个 pass 之后
> 自动跑 `LLVMVerifyModule`，损坏即点名（`GOPROTECT_DUMP_IR=/path` 可在失败时
> 落盘 IR）。

## 主要能力

- **常量混淆（const-obf）**：整数常量按强度比例替换为 `(x^k)^k` 或 `(x−k)+k`
  恒等链；私有字符串全局（使用者全为指令）替换为可写密文全局并导出
  `__gp_str_regions` 区域表，运行时经 `__goprotect_decrypt_strings` 原位解密
  ——产物 IR/.rodata 中不再有明文。密钥经 alloca+store+load 栈槽中转，
  规避 IRBuilder 的常数折叠（否则包裹会静默消失）。
- **控制流平坦化（cf-flatten）**：真实终结器分析与重写，条件边拆双跳板，
  phi/switch 边界函数保守跳过。
- **虚拟化（virtualize）**：
  - 支持 **void 与 i32 返回**、**全 i32 参数（≤4 个）** 的函数；i64 等截断
    风险类型与 phi/call/访存函数保守跳过（宁可不虚拟化，绝不产出算错的字节码）；
  - **旧函数体整体擦除**，替换为对统一入口的调用桩——产物不含原指令；
  - 统一入口 `__goprotect_vm_entry_encrypted(i8* bc, i8* meta, i32 a0..a3) → i32`
    （vm_a/vm_b 保留同签名别名）；VM 选择烘焙在字节码数据里，自定义 VM 名
    不会产生链接陷阱；
  - 每次构建随机化 opcode 映射（`opcodes`：助记符 → 随机字节），字节码 XOR
    加密，密钥 = `静态片段 ^ key_pad`（静态片段经 `goprotect_set_static_key`
    注册，默认 **0x5A**——与配置 `static_key` 留空时的缺省一致；显式配置时
    应用侧必须注册同值）。
- **安全钩子**：`__goprotect_check_integrity`（注册区域 FNV-1a 基线复检）与
  `__goprotect_anti_debug`（TracerPid/maps/模拟器 + frida 端口探测）。

## 运行时

- 解释器与解密：`runtime/src/vm_entry.c`（按元数据解码/解密/播种实参/执行，
  `RET_VALUE` 回传返回值）。
- 字符串解密：`runtime/src/hooks.c`。
- 完整性与反调试：`runtime/src/integrity.c` / `antidebug.c`。
- host 验证：`lli --extra-object=runtime.o out.bc`（语义退出码断言），CI 中
  由 llvm job 自动执行（`go test -tags llvm` + 共享 runtime 对象）。
- NDK 集成：`runtime/CMakeLists.txt`；反调试经 `__goprotect_hook` 的自动
  触发需编译期定义 `GOPROTECT_RUNTIME_ENABLE_CHECKS`（默认关闭，零开销）。

## 配置要点

参见 `config/example.yml`：

- `functions.allow/deny`：函数名白/黑名单；`allow` 为空表示全部允许。
- `passes.*`：启用/关闭各个 Pass。
- `obfuscation.*`：强度与触发概率（`substitute_intensity` 1–10、`flatten_ratio`、
  `virtualize_ratio` 百分比）。
- `vmp.*`：多 VM 配置与 `static_key`（见上：显式配置需运行时注册同值）。
- `report.path`：输出 JSON 报告，记录哪些函数被处理及所用 Pass。
- `debug.dump_cfg`：等价 `--dump-cfg`。

## 集成 NDK 编译

1. 使用 clang 生成 IR：`clang -c -emit-llvm foo.c -o foo.bc`（或直接用 `.ll`）。
2. 运行 `goprotect`：`./goprotect --config config.yml --input foo.bc -o foo.obf.bc`。
3. 继续正常链接：`clang foo.obf.bc -shared -o libfoo.so ...`（runtime 与应用
   一起链接，`goprotect_set_static_key` 在启动早期注册静态密钥片段）。

## 目录结构

- `cmd/goprotect/`：CLI 入口。
- `config/`：配置解析与示例。
- `passes/`：各 IR Pass（虚拟化、插桩、混淆、安全钩子；`passes/vmp/` 为字节码
  编译器）。
- `llvmwrap/`：Go 版 LLVM C API 轻量封装（`-tags llvm` 时启用，否则 mock）。
- `passes/testdata/`：llvm-tagged 集成测试夹具。
- `report/`：变换报告输出。
- `docs/`：使用文档、设计笔记与 [ROADMAP](ROADMAP.md)。
