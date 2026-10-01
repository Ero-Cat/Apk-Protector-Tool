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

### LLVM 集成测试（需本机 LLVM；CI 的 llvm job 自动执行）
```bash
# 需要 pkg-config 能找到 llvm（Ubuntu 的 llvm-dev 不带 llvm.pc，
# 用 scripts/ci-llvm-pkgconfig.sh 生成 shim）
PKG_CONFIG_PATH=<llvm.pc 目录> CGO_ENABLED=1 go test -tags llvm ./llvmwrap/... ./passes/...

# 语义级断言（lli 执行 main 的退出码在 pass 前后一致）需要 runtime 对象，
# 缺失则相关用例自动 Skip：
GOPROTECT_TEST_RUNTIME_OBJ=$PWD/runtime.o PKG_CONFIG_PATH=... CGO_ENABLED=1 \
  go test -tags llvm ./passes/...
```

- 夹具放 `passes/testdata/*.ll`，覆盖 void/非 void/phi/switch/字符串全局等形态
- 断言分层：结构断言（`mod.String()` 含/不含特定模式）+ 语义断言（lli 退出码）
- **mock 零值陷阱**：`llvmwrap.BasicBlock{}` 等零值仅在默认（mock）构建下安全，
  `-tags llvm` 下会解引用空指针段错误——此类测试必须
  `if llvmwrap.HasNative() { t.Skip(...) }`（见 vmp/compiler_test.go 示例）

## 测试文件规范

### 命名
- 测试文件：`<name>_test.go`
- LLVM 集成测试：`<name>_llvm_test.go` + 文件头 `//go:build llvm`

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

## Mock 外部二进制（CommandRunner，已落地于 internal/app/runner.go）

### 原则
- 不在单元测试中调用真实的 `zipalign`、`apksigner`、`keytool`、`adb`
- 一律经 `Tool.Runner` 注入（`NewTool` 默认 `realRunner{}`）

### 实际接口（勿重新发明）
```go
type CommandRunner interface {
    Run(ctx context.Context, bin string, args []string, env map[string]string,
        timeout time.Duration, stdout, stderr io.Writer) error
    Output(bin string, args []string) ([]byte, error)
    LookPath(bin string) (string, error)
}
```

- 范例见 `internal/app/tool_orchestration_test.go` 的 fakeRunner
- fake 需仿真外部工具的**产物落盘副作用**：zipalign 复制 倒数第二参数→最后参数、
  apksigner sign 按 `--out` 落盘（输入是最后一个参数）、keytool 生成 keystore
  文件——后续阶段会检查输出文件存在，不落盘则链路断
- 编排断言口径：工具缺失 → preflight 即失败（`report == nil`）；中段失败 →
  前序步骤保持 completed、失败步骤 failed、后续步骤不出现

## C 运行时测试

- `runtime/tests/test_*.c` 单翻译区模式：`#include "../src/<file>.c"`
- 编译命令与 CI runtime job 一致：
  `clang -std=c11 -Wall -Wextra -Werror -I runtime/include runtime/tests/test_x.c -o /tmp/test_x`
- 新增 C 源文件/测试时，同步更新 `.github/workflows/ci.yml`（runtime job）与
  `scripts/e2e-basic.sh` 的可选 clang 段
- Go↔C 契约（opcode 表/解释器覆盖/共享向量）由构建期测试锚定
  （`passes/vmp/opcodes_consistency_test.go` 等），改 ISA 必须 Go/C 两侧同步改
- `runtime/android/` 的加密核心 host 可测：test_aes_gcm.c（NIST + Go 交叉
  向量）、test_dex_decrypt.c（密钥拼装/解密/容量防护）

## 测试数据

如需测试数据文件：
- 放置于 `testdata/` 目录
- 使用小型 fixture，避免大文件
- 示例：`passes/testdata/arith.ll`

## CI 集成

修改代码后必须确保：

```bash
// turbo-all
go build ./...
go test ./...
gofmt -l . | grep -q . && exit 1 || true
go vet ./...
```

涉及 LLVM/运行时的改动还需：`go test -tags llvm ./llvmwrap/... ./passes/...`
与 `bash scripts/e2e-basic.sh`（本地有 clang 时含 C 单测）。
