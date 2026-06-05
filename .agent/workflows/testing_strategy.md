---
description: 项目测试策略与规范
---

# Testing Strategy

## 测试框架

使用 Go 内置 `testing` 包，无第三方测试依赖。

## 测试命令

```bash
// turbo
go test ./...
```

### 带覆盖率
```bash
go test -cover ./...
```

### 详细输出
```bash
go test -v ./...
```

## 测试文件规范

### 命名
- 测试文件：`<name>_test.go`
- 测试函数：`Test<FuncName>(t *testing.T)`

### 位置
- 与被测代码放在同一目录
- 示例：`internal/app/tool_test.go`

## 表驱动测试模板

```go
func TestDeriveKey(t *testing.T) {
    tests := []struct {
        name    string
        secret  string
        wantLen int
        wantErr bool
    }{
        {"empty secret", "", 32, false},
        {"normal secret", "mx-guard-v1", 32, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            key, _, err := deriveKey(tt.secret)
            if (err != nil) != tt.wantErr {
                t.Errorf("deriveKey() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if len(key) != tt.wantLen {
                t.Errorf("deriveKey() key len = %v, want %v", len(key), tt.wantLen)
            }
        })
    }
}
```

## Mock 外部二进制

### 原则
- 不在单元测试中调用真实的 `zipalign`、`apksigner`、`adb`
- 通过接口抽象或路径注入实现 mock

### 示例
```go
type CommandRunner interface {
    Run(ctx context.Context, bin string, args []string) error
}

// 生产代码使用 realRunner
// 测试代码使用 mockRunner
```

## 测试数据

如需测试数据文件：
- 放置于 `testdata/` 目录
- 使用小型 fixture，避免大文件
- 示例：`testdata/minimal.apk`

## CI 集成

修改代码后必须确保：

```bash
// turbo-all
go build ./...
go test ./...
gofmt -l . | grep -q . && exit 1 || true
go vet ./...
```
