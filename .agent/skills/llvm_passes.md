---
name: LLVM Passes
description: LLVM/IR 混淆 Pass 开发技能，包含接口定义与实现模板
---

# LLVM Passes Skill

## 概述

本项目的 LLVM IR 混淆通过 `passes/` 包实现，使用 Pipeline 模式顺序执行多个 Pass。
输入支持 `.bc` 位码与 `.ll` 文本 IR（按扩展名分发）；每个 Pass 之后自动跑
`LLVMVerifyModule`，失败时设 `GOPROTECT_DUMP_IR=/path` 落盘 IR 排查。

---

## Pass 接口

```go
// passes/pipeline.go
type Pass interface {
    Name() string
    Run(m *llvmwrap.Module) error
}
```

---

## 现有 Pass 列表（按执行顺序）

| 顺序 | Pass | 文件 | 功能 |
|------|------|------|------|
| 1 | VirtualizePass | `virtualize.go` | **必须最先执行**：编译原始函数体为 VM 字节码，旧体整体擦除，替换为统一入口调用桩（void/i32 返回、≤4 个 i32 参数） |
| 2 | EntryExitPass | `entry_exit.go` | 入口/每个 ret 前调用 `__goprotect_hook` |
| 3 | ConstSplitPass | `const_split.go` | 常量拆分 |
| 4 | InstrSubPass | `instr_sub.go` | 按强度用恒等链包裹整型操作数 |
| 5 | CFFlattenPass | `cf_flatten.go` | 调度器式控制流平坦化 |
| 6 | ConstObfPass | `const_obf.go` | 整数常量恒等改写 + 私有字符串全局加密（运行时原位解密） |
| 7 | SecurityHooksPass | `security_hooks.go` | 入口调用完整性校验/反调试 |

> **为什么 virtualize 排第一**：任何插桩/包裹都会给函数体加进 alloca、call
> 等指令，而 VM 编译器对它们保守跳过——virtualize 若不在最前，默认配置下
> 永远虚拟化不了任何函数（历史上真实发生过，见 ROADMAP P2.3 修正记录）。

---

## 新 Pass 实现模板

```go
package passes

import (
    "math/rand"
    "github.com/Ero-Cat/Apk-Protector-Tool/config"
    "github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
    "github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// MyNewPass：描述 Pass 功能。
type MyNewPass struct {
    Cfg    *config.Config
    Rand   *rand.Rand
    Report *report.Report
}

func (p *MyNewPass) Name() string { return "my_new_pass" }

func (p *MyNewPass) Run(m *llvmwrap.Module) error {
    for _, fn := range m.Functions() {
        // 跳过过滤函数
        if skipFunction(fn.Name(), p.Cfg) {
            continue
        }

        // 实现变换逻辑
        for _, bb := range fn.BasicBlocks() {
            // 处理基本块
        }
    }

    p.Report.AddMessage("my_new_pass: completed")
    return nil
}
```

---

## 注册到 Pipeline

在 `passes/pipeline.go` 的 `BuildPipeline` 函数中添加（注意与 virtualize 的
顺序约束）：

```go
if cfg.Passes.MyNewPass {
    passes = append(passes, &MyNewPass{Cfg: cfg, Rand: rnd, Report: rpt})
}
```

并在 `config/config.go` 的 `Passes` 结构体中添加配置开关：

```go
Passes struct {
    // ... 现有字段
    MyNewPass bool `json:"my_new_pass" yaml:"my_new_pass"`
}
```

---

## ⚠️ 常数折叠陷阱（改常量/包裹类 Pass 必读）

IRBuilder 会对**全常量操作数**的二元运算做常数折叠：`xor(3, k)` 直接折回
一个常量，"包裹指令"从未生成、SetOperand 设回去等于无操作——编译不报错、
语义不变、混淆静默消失。**密钥必须经 alloca+store+load 栈槽中转**（load 出
来的值非常量，链得以保留）：直接用 `passes/wrap.go` 的 `wrapOperand` /
`keySlots`，不要手写直接 xor。同理：

- phi 与终结器（switch 的 case 值必须是常量）不做操作数包裹；
- 对指针做 xor 在 opaque pointer 下是非法 IR。

---

## llvmwrap API 速查

### 模块操作
```go
mod, _ := llvmwrap.ParseBitcode(path) // .bc 或 .ll
defer mod.Dispose()
mod.WriteBitcode(outputPath)          // 输出恒为位码
mod.Verify()                          // verifier；Pipeline 每 pass 后自动跑
mod.String()                          // 文本 IR（断言/调试）
```

### 函数与 CFG
```go
for _, fn := range mod.Functions() {
    fn.ReturnType()                    // void 判断用 .Equal(llvmwrap.VoidType())
    fn.ParamCount(); fn.Param(i)       // 参数（P2.3 ABI：全 i32 且 ≤4）
    for _, bb := range fn.BasicBlocks() {
        bb.Terminator()                // 终结器指令
        bb.Successors()                // 后继块（br/condbr/switch 通用）
        bb.Delete()                    // 删除已清空的块
    }
}
inst.Opcode()                          // "add"/"br"/"call"...（br 槽位经运行期探测，跨版本安全）
inst.Operands(); inst.SetOperand(i, v) // 改写操作数（包裹用这个，不要 RAUIW）
inst.EraseFromParent()
```

### 全局与使用者（字面量改写用）
```go
for _, g := range mod.Globals() {
    g.IsPrivateLinkage(); g.IsGlobalConstant()
    data, ok := g.Initializer().ConstantDataArrayBytes() // c"..." 明文字节
    for _, u := range g.Users() { u.IsInstruction(); u.AsInstruction() }
    mod.AddGlobalBytes(name, enc)   // 私有、可写的密文全局
}
mod.EmitStrRegionsTable("__gp_str_regions", regions) // C 侧解密表
```

### IR Builder
```go
builder := llvmwrap.NewBuilderAtEnd(basicBlock)
defer builder.Dispose()

builder.CreateCall(fn, args)
builder.CreateRet(v)                    // 带返回值；CreateRetVoid() 为 void
builder.CreateBitCast(val, typ, name)
builder.CreateStore(val, ptr); builder.CreateLoad(typ, ptr, name)
```

---

## VMP 虚拟化

### 配置
```yaml
vmp:
  enable_multi_vm: true
  vms:
    - {name: vm_a, isa: A}
    - {name: vm_b, isa: B}
  levels:
    normal: vm_a
    sensitive: vm_b
    critical: vm_b
  static_key: ""   # 留空 = 0x5A，与 C 运行时缺省一致（推荐）
  runtime_key_hint: env_device_seed
```

> **static_key 契约**：留空时两侧统一用 0x5A。**显式配置**时应用侧必须在
> 启动早期 `goprotect_set_static_key(<末字节>)` 注册同一值，否则字节码永远
> 解不开（历史上默认 `c0ffee42`（0x32）与运行时 0x5A 错配过，见 ROADMAP P2.3）。

### 运行时约定
- 统一入口符号：`__goprotect_vm_entry_encrypted(i8* bc, i8* meta, i32 a0..a3) → i32`
  （VM 选择烘焙在字节码的随机化映射与 key_pad 里，与符号无关；vm_a/vm_b
  保留为同签名别名）
- 字节码全局：`__gp_bc_<fn_name>`；元数据全局：`__gp_bc_meta_<fn_name>`
- host 验证：`lli --extra-object=runtime.o out.bc`（退出码断言）

---

> [!TIP]
> 开发新 Pass 前，先确认是否可通过调整现有 Pass 的 ratio/intensity 参数实现目标；
> 涉及 IR 变换的改动必须补 `passes/*_llvm_test.go`（`//go:build llvm`）夹具断言。
