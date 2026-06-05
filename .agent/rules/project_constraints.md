---
trigger: always_on
---

# Project Constraints

本项目的硬性约束，AI Agent **不可违反**。

## 代码约束

### 1. 格式化必须通过
```bash
gofmt -l .   # 必须无输出
go vet ./... # 必须无警告
```

### 2. JSON Struct Tags
- **禁止**：使用 CamelCase 或 PascalCase
- **必须**：使用 snake_case

```go
// ❌ 禁止
Field string `json:"fieldName"`

// ✅ 必须
Field string `json:"field_name"`
```

### 3. 包级变量
- **禁止**：业务逻辑使用包级变量
- **例外**：config defaults 函数内的默认值

### 4. 外部二进制调用
- **禁止**：硬编码绝对路径
- **必须**：通过配置或参数注入路径

```go
// ❌ 禁止
exec.Command("/usr/local/bin/zipalign", ...)

// ✅ 必须
exec.Command(cfg.ZipalignPath, ...)
```

## 安全约束

### 1. 敏感信息
- **禁止**：commit keystore 密码、API 密钥到代码
- **必须**：使用 `config.json` 本地覆盖或环境变量

### 2. 路径处理
- **禁止**：用户配置中使用硬编码用户目录 (如 `/Users/xxx/`)
- **推荐**：使用 `~` 或相对路径，运行时展开

## 构建约束

### 1. LLVM 功能
- **条件**：需要 `-tags llvm` 编译标签
- **回退**：`llvmwrap/mock.go` 提供非 LLVM 模式

### 2. Go 版本
- **最低**：Go 1.25
- **构建命令**：`GOCACHE=$(pwd)/.gocache go build ./cmd/protector`

## 输出约束

### 1. 临时文件
- 放置于 `build/temp` 或 `tmp/`
- **禁止**：提交到版本控制

### 2. 最终产物
- APK 输出到 `dist/`
- 报告输出到 `dist/` 或 `report/`

### 3. 清理命令
```bash
rm -rf dist build/temp tmp protector.tmp
```
