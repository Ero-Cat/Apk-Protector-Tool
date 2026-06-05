---
trigger: always_on
---

# Code Style Guide

本规则从现有代码库逆向分析得出，AI Agent 生成代码时**必须**遵循。

## 命名约定

| 场景 | 规范 | 示例 |
|------|------|------|
| Go 类型/函数 | CamelCase | `VirtualizePass`, `runProtections` |
| JSON struct tags | snake_case | `json:"input_apk"` |
| YAML struct tags | snake_case | `yaml:"encryption_secret"` |
| CLI 参数 | kebab-case | `-protect-compress`, `-key-alias` |
| 包名 | 小写单词 | `app`, `config`, `passes` |
| 常量 | CamelCase 或 全大写 | `VoidType`, `DefaultAlignment` |

## 函数设计

1. **单一职责**：每个函数只做一件事，保持 pipeline 步骤显式命名
2. **错误返回**：使用 `(result, error)` 返回模式
3. **错误包装**：使用 `fmt.Errorf("%s: %w", context, err)` 保留错误链
4. **避免全局状态**：除 config defaults 外不使用包级变量

```go
// ✅ 正确示例
func (t *Tool) runZipalign(ctx context.Context, apk string) (string, error) {
    if err := runCommand(ctx, cfg.Path, args); err != nil {
        return "", fmt.Errorf("zipalign: %w", err)
    }
    return outputPath, nil
}

// ❌ 错误示例
func zipalign(apk string) string {
    // 缺少 context, 缺少错误返回, 硬编码路径
}
```

## 注释规范

- **Docstring**：使用中文描述复杂逻辑，英文描述公开 API
- **格式**：`// FuncName 描述...` 紧贴函数声明
- **内联注释**：解释"为什么"而非"是什么"

```go
// VirtualizePass：将部分 void 函数替换为 VM 入口跳板，并把生成的字节码以全局数组形式存入模块。
// 解释器/VM 的运行时实现需要在 Android 侧单独提供。
type VirtualizePass struct { ... }
```

## Struct Tags

JSON 与 YAML 必须同时声明，保持一致：

```go
type Config struct {
    Input  string `json:"input" yaml:"input"`
    Output string `json:"output" yaml:"output"`
}
```

## 代码格式化

- **强制工具**：`gofmt -w .`
- **缩进**：Tab（由 gofmt 保证）
- **行宽**：无硬性限制，但避免超过 120 字符
