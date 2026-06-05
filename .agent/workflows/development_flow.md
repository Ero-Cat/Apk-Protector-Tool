---
description: 本项目的标准开发流程，从编译到格式化验证
---

# Development Flow

## 1. 编译 CLI

### protector (APK 加固工具)
```bash
// turbo
GOCACHE=$(pwd)/.gocache go build ./cmd/protector
```

### goprotect (LLVM 混淆工具)
```bash
GOCACHE=$(pwd)/.gocache go build -tags llvm ./cmd/goprotect
```

> [!NOTE]
> `goprotect` 需要系统安装 LLVM 并配置 `llvm-config`

## 2. 运行测试
```bash
// turbo
go test ./...
```

## 3. 格式化代码
```bash
// turbo
gofmt -w .
```

## 4. 静态分析
```bash
// turbo
go vet ./...
```

## 5. 运行工具 (示例)

### APK 加固
```bash
./protector \
  -input app-release.apk \
  -output dist/app-protected.apk \
  -protect \
  -protect-compress \
  -zipalign "$BT/zipalign" \
  -apksigner "$BT/apksigner" \
  -keystore sign/release.keystore \
  -store-pass 1234567 \
  -key-alias testalias
```

### LLVM 混淆
```bash
./goprotect \
  -config config/example.yml \
  -input input.bc \
  -o output.bc \
  -level medium
```

## 6. 清理临时文件
```bash
rm -rf dist build/temp tmp protector.tmp
```

## 快速验证脚本
```bash
#!/bin/bash
set -e
GOCACHE=$(pwd)/.gocache go build ./cmd/protector
go test ./...
gofmt -l . | grep -q . && echo "需要格式化" && exit 1
go vet ./...
echo "✓ 所有检查通过"
```
