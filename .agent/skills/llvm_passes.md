---
name: LLVM Passes
description: LLVM/IR 混淆 Pass 开发技能，包含接口定义与实现模板
---

# LLVM Passes Skill

## 概述

本项目的 LLVM IR 混淆通过 `passes/` 包实现，使用 Pipeline 模式顺序执行多个 Pass。

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

## 现有 Pass 列表

| Pass | 文件 | 功能 |
|------|------|------|
| EntryExitPass | `entry_exit.go` | 函数入口/出口钩子插入 |
| ConstSplitPass | `const_split.go` | 常量拆分 |
| InstrSubPass | `instr_sub.go` | 指令替换 |
| CFFlattenPass | `cf_flatten.go` | 控制流平坦化 |
| ConstObfPass | `const_obf.go` | 常量保护钩子 |
| VirtualizePass | `virtualize.go` | VMP 虚拟化 |
| SecurityHooksPass | `security_hooks.go` | 安全钩子 (完整性/反调试) |

---

## 新 Pass 实现模板

```go
package passes

import (
    "math/rand"
    "protector-tool/config"
    "protector-tool/llvmwrap"
    "protector-tool/report"
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

在 `passes/pipeline.go` 的 `BuildPipeline` 函数中添加：

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

## llvmwrap API 速查

### 模块操作
```go
mod, _ := llvmwrap.ParseBitcode(path)
defer mod.Dispose()
mod.WriteBitcode(outputPath)
```

### 函数遍历
```go
for _, fn := range mod.Functions() {
    fmt.Println(fn.Name())
    for _, bb := range fn.BasicBlocks() {
        // 处理基本块
    }
}
```

### 类型
```go
llvmwrap.VoidType()
llvmwrap.IntType(bits)      // IntType(8), IntType(32)
llvmwrap.PointerType(elem)
```

### 全局变量
```go
gv := mod.AddGlobalString("name", data)
ptr := gv.AsValue()
```

### IR Builder
```go
builder := llvmwrap.NewBuilderAtEnd(basicBlock)
defer builder.Dispose()

builder.CreateCall(fn, args)
builder.CreateRetVoid()
builder.CreateBitCast(val, typ, name)
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
  static_key: c0ffee42
  runtime_key_hint: env_device_seed
```

### 运行时约定
- 入口符号：`__goprotect_vm_entry_encrypted_<vm_name>`
- 字节码全局：`__gp_bc_<fn_name>`
- 元数据全局：`__gp_bc_meta_<fn_name>`

---

> [!TIP]
> 开发新 Pass 前，先确认是否可通过调整现有 Pass 的 ratio/intensity 参数实现目标。
