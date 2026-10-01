---
trigger: always_on
---

# Tech Stack Conventions

项目使用的技术栈最佳实践指南。

## Go 1.25

### 模块管理
```go
module github.com/Ero-Cat/Apk-Protector-Tool

go 1.25

require gopkg.in/yaml.v3 v3.0.1
```

### 错误处理
- 使用 `errors.Is()` / `errors.As()` 进行错误判断
- 使用 `fmt.Errorf("...: %w", err)` 包装错误

## gopkg.in/yaml.v3

### 配置解析
```go
import "gopkg.in/yaml.v3"

var cfg Config
if err := yaml.Unmarshal(data, &cfg); err != nil {
    return fmt.Errorf("parse yaml: %w", err)
}
```

### Struct Tags
同时声明 `json` 和 `yaml` 标签以支持双格式：
```go
type Config struct {
    Input string `json:"input" yaml:"input"`
}
```

## LLVM C API 封装 (llvmwrap/)

### 架构
```
llvmwrap/
├── types.go    # 类型定义 (Module, Function, Value, etc.)
├── native.go   # LLVM C API 真实实现 (需 -tags llvm)
└── mock.go     # 模拟实现 (无 LLVM 时使用)
```

### 检测 LLVM 可用性
```go
if !llvmwrap.HasNative() {
    log.Fatalf("需要 LLVM 支持，请使用 -tags llvm 重新编译")
}
```

### 模块操作
```go
mod, err := llvmwrap.ParseBitcode(path)
defer mod.Dispose()

for _, fn := range mod.Functions() {
    // 处理函数
}

mod.WriteBitcode(outputPath)
```

## Pass 接口模式

### 定义
```go
type Pass interface {
    Name() string
    Run(m *llvmwrap.Module) error
}
```

### 实现模板
```go
type MyPass struct {
    Cfg    *config.Config
    Rand   *rand.Rand
    Report *report.Report
}

func (p *MyPass) Name() string { return "my_pass" }

func (p *MyPass) Run(m *llvmwrap.Module) error {
    for _, fn := range m.Functions() {
        // 变换逻辑
    }
    p.Report.AddMessage("my_pass completed")
    return nil
}
```

### Pipeline 构建
```go
pipeline := passes.BuildPipeline(cfg, report)
if err := pipeline.Run(module); err != nil {
    return fmt.Errorf("pipeline: %w", err)
}
```

## Android Build Tools

### 路径约定
- zipalign: `~/Library/Android/sdk/build-tools/<version>/zipalign`
- apksigner: `~/Library/Android/sdk/build-tools/<version>/apksigner`

### 调用模式
```go
func runCommand(ctx context.Context, bin string, args []string, env map[string]string, timeout time.Duration) error
```

所有外部工具调用都通过统一的 `runCommand` 封装，支持：
- Context 取消
- 超时控制
- 环境变量注入
