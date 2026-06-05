---
name: Core Utils
description: 项目核心工具函数，AI Agent 应优先复用而非重复实现
---

# Core Utils Skill

## 概述

本项目已封装的核心工具函数，**禁止重复造轮子**。

---

## 文件操作 (internal/app/tool.go)

### copyFile
复制文件，保留权限。
```go
func copyFile(src, dst string) error
```

### fileExists
检查文件是否存在。
```go
func fileExists(path string) bool
```

### computeSHA256
计算文件 SHA256 哈希。
```go
func computeSHA256(path string) (string, error)
```

---

## 命令执行 (internal/app/tool.go)

### runCommand
统一的外部命令执行封装，支持 context、超时、环境变量。
```go
func runCommand(ctx context.Context, bin string, args []string, env map[string]string, timeout time.Duration) error
```

**示例**：
```go
err := runCommand(ctx, cfg.ApksignerPath, []string{"sign", "--ks", ks}, nil, 5*time.Minute)
```

---

## 模板处理 (internal/app/tool.go)

### applyTemplate
替换字符串中的 `{{key}}` 占位符。
```go
func applyTemplate(value string, vars map[string]string) string
```

### applyTemplates
批量替换字符串切片。
```go
func applyTemplates(values []string, vars map[string]string) []string
```

### expandEnv
替换 map 值中的模板变量。
```go
func expandEnv(env map[string]string, vars map[string]string) map[string]string
```

---

## 配置加载 (config/config.go)

### config.Load
加载 YAML 或 JSON 配置文件，自动填充默认值。
```go
func Load(path string) (*Config, error)
```

**双格式支持**：根据文件扩展名自动选择解析器。

---

## 报告生成 (report/)

### report.New
创建新的报告实例。
```go
func New() *Report
```

### report.Write
将报告写入 JSON 文件。
```go
func (r *Report) Write(path string) error
```

### report.AddMessage
添加通用消息。
```go
func (r *Report) AddMessage(msg string)
```

### report.MarkVirtualized
标记函数已被虚拟化。
```go
func (r *Report) MarkVirtualized(fn string)
```

---

## 压缩/解压 (internal/app/protections.go)

### unzipArchive
解压 ZIP 文件到目标目录。
```go
func unzipArchive(src, dest string) error
```

### zipDirectory
将目录压缩为 ZIP 文件。
```go
func zipDirectory(srcDir, dest string) error
```

### deflateBytes
使用 Deflate 压缩字节数组。
```go
func deflateBytes(data []byte) ([]byte, error)
```

---

## 加密 (internal/app/protections.go)

### deriveKey
从密钥种子派生 AES-256 密钥。
```go
func deriveKey(secret string) ([]byte, string, error)
```

### encryptBytes
AES-GCM 加密。
```go
func encryptBytes(data, key []byte) ([]byte, []byte, error)
```

---

> [!IMPORTANT]
> 新增功能前，先检查上述函数是否已覆盖需求。复用现有工具可保持代码一致性。
