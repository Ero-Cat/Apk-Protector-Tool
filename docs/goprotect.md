# goprotect 编译期保护工具

本文介绍如何在 Android NDK/LLVM 构建流水线中使用 `goprotect` 对本地库的 LLVM IR/bitcode 进行编译期混淆与安全加固。

## 快速使用

```bash
# 假设已安装匹配版本的 LLVM，并开启 cgo：
go build -tags llvm ./cmd/goprotect

# 对输入 bitcode 进行处理
./goprotect --config config/example.yml --input input.bc -o output.bc
```

- `--config`：YAML/JSON 配置文件，控制启用的 Pass、函数白名单/黑名单及混淆强度。
- `--input` / `-o`：输入与输出 bitcode/IR 路径。
- `--level`：可临时覆盖配置中的 `obfuscation.level`（low/medium/high）。
- `--dump-cfg`：输出前后 CFG 的 DOT（预留开关，目前存根）。

## 主要能力（编译侧）

- **入口/出口插桩**：函数入/出调用 `__goprotect_hook` 预留钩子。
- **常量拆分与指令替换**：将立即数拆为多步，加/异或双向包裹指令结果。
- **简易控制流扰动**：插入虚假基本块，后续可扩展为调度器式扁平化。
- **常量保护钩子**：入口调用 `__goprotect_decrypt_strings`，为运行时解密留接口。
- **虚拟化存根**：为部分 `void` 函数生成字节码 blob，并将函数体替换为 `__goprotect_vm_entry` 调用。
- **安全钩子**：在关键函数入口调用 `__goprotect_check_integrity` 与 `__goprotect_anti_debug`。

### 进阶 VMP 支持（本次新增）
- **多 VM/分级保护**：可配置 VM_A / VM_B 不同 ISA，按 normal/sensitive/critical 分类选择 VM；`enable_multi_vm: false` 可折叠为单 VM。
- **按构建随机化**：每次构建为每个 VM 生成随机 opcode 映射并写入元数据（`opcodes`：标准助记符 → 随机化字节），运行时据此构建解码表。
- **字节码加密**：生成的字节码以 XOR key 加密，运行时通过 `__goprotect_vm_entry_encrypted_<vm>(bytecode, meta)` 解密执行；密钥拆分为 `注册静态片段 ^ key_pad`（静态片段经 `goprotect_set_static_key` 注入，默认 0x5A），不随 APK 元数据分发。
- **元数据输出**：为每个虚拟化函数生成 meta 全局常量（包含 VM 名、build nonce、`bytecode_len`、`encrypted`、`key_pad`、`param_count`、opcode 解码表与 `ext_funcs`）；解释器先经解码表还原标准 opcode 再执行，Go/C 两侧定义由构建期测试校验一致。

运行时解密/VM 解释器已随 `runtime/` 提供（见 `runtime/src/vm_entry.c` 与 `hooks.c`）；真机端到端联调仍需配合 NDK 集成（ROADMAP P3/P4）。

## 配置要点

参见 `config/example.yml`：

- `functions.allow/deny`：函数名白/黑名单；`allow` 为空表示全部允许。
- `passes.*`：启用/关闭各个 Pass。
- `obfuscation.*`：强度与触发概率（如 `flatten_ratio`、`virtualize_ratio`）。
- `report.path`：输出 JSON 报告，记录哪些函数被处理及所用 Pass。

## 集成 NDK 编译

1. 使用 clang 生成 `.bc`：`clang -c -emit-llvm foo.c -o foo.bc`。
2. 运行 `goprotect`：`./goprotect --config config.yml --input foo.bc -o foo.obf.bc`。
3. 继续正常链接：`clang foo.obf.bc -shared -o libfoo.so ...`

## 目录结构（新增部分）

- `cmd/goprotect/`：CLI 入口。
- `config/`：配置解析与示例。
- `passes/`：各 IR Pass（插桩、混淆、虚拟化、安全钩子）。
- `llvmwrap/`：Go 版 LLVM C API 轻量封装（`-tags llvm` 时启用）。
- `report/`：变换报告输出。
- `docs/`：使用文档与设计笔记。
