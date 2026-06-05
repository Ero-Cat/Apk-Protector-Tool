# Repository Guidelines

## Project Tech Stack Summary

| Dimension | Details |
|-----------|---------|
| **Language** | Go 1.25 |
| **Dependencies** | `gopkg.in/yaml.v3` |
| **Architecture** | Layered (CLI → Internal → Config/Passes) |
| **Entry Points** | `cmd/protector` (APK), `cmd/goprotect` (LLVM) |
| **Config Format** | JSON + YAML dual-mode |
| **External Tools** | Android build-tools (zipalign, apksigner), LLVM (optional) |

## Agent Roles Definition

### APK_Hardening_Expert
**Scope**: `internal/app/`, `sign/`, APK 加固流水线
**Responsibilities**:
- DEX 加密与压缩 (AES-GCM + Deflate)
- 包名随机化、伪加固标记
- zipalign 对齐、签名、验签
- 静态扫描 (加固特征、密钥泄露检测)

### LLVM_Obfuscation_Specialist
**Scope**: `passes/`, `llvmwrap/`, `config/`
**Responsibilities**:
- IR Pass 开发 (控制流平坦化、常量拆分、VMP 虚拟化)
- LLVM C API 封装维护
- VMP 多 VM 随机化与字节码加密
- 混淆强度配置调优

### CLI_Maintainer
**Scope**: `cmd/protector/`, `cmd/goprotect/`
**Responsibilities**:
- Flag 解析与 config 覆盖逻辑
- 用户体验改进 (帮助信息、错误提示)
- 入口函数维护

---

## Project Structure & Module Organization
- `cmd/protector`: APK 加固 CLI；flag 解析与 config 覆盖
- `cmd/goprotect`: LLVM 混淆 CLI；读取 IR，执行 Pass 序列
- `internal/app`: 核心 pipeline（config 解析、扫描、保护、签名、报告）
- `passes/`: LLVM/IR 混淆 Pass 实现
- `llvmwrap/`: LLVM C API Go 封装（native + mock 双实现）
- `config/`: 配置结构定义与加载
- `config.json.example`: 复制为 `config.json` 使用；secrets 不要提交
- `sign/`: 本地 keystore 材料；视为敏感目录
- `dist/`, `tmp/`, `build/`: 生成输出；不纳入版本控制

## Build, Test, and Development Commands
```bash
# 编译 APK 加固 CLI
GOCACHE=$(pwd)/.gocache go build ./cmd/protector

# 编译 LLVM 混淆 CLI (需 LLVM)
GOCACHE=$(pwd)/.gocache go build -tags llvm ./cmd/goprotect

# 运行测试
go test ./...

# 格式化与静态检查
gofmt -w .
go vet ./...
```

## Coding Style & Naming Conventions
- **Go 风格**: 使用标准 gofmt，Tab 缩进
- **类型/函数**: CamelCase (`VirtualizePass`, `runProtections`)
- **JSON 字段**: snake_case (`input_apk`, `encryption_secret`)
- **CLI 参数**: kebab-case (`-protect-compress`, `-key-alias`)
- **注释**: 中英混合；核心逻辑使用中文 docstring
- 详见 `.agent/rules/code_style_guide.md`

## Testing Guidelines
- 使用 Go 内置 `testing` 包，表驱动测试风格
- Mock 外部二进制 (`zipalign`, `apksigner`, `adb`)
- 测试文件放置于同目录，命名 `*_test.go`
- 详见 `.agent/workflows/testing_strategy.md`

## Commit & Pull Request Guidelines
- 遵循 Conventional Commits (`fix:`, `feat:`, `docs:`)
- Subject 使用祈使句，不超过 70 字符
- PR 描述变更内容、涉及入口、新增 flag/config 字段
- 如输出产物变更，附带示例命令与结果文件名

## Security & Configuration Tips
- Keystore 密码使用 env 或本地 `config.json`，不要提交
- 外部工具路径通过配置注入，不硬编码
- 清理命令：`rm -rf dist build/temp tmp protector.tmp`

---

## VibeCoding Integration

本项目已配置 Agent 辅助开发体系：

| 目录 | 用途 |
|------|------|
| `.agent/rules/` | 代码风格、项目约束、技术栈规范 |
| `.agent/workflows/` | 开发流程、测试策略 |
| `.agent/skills/` | 核心工具函数、LLVM Pass 开发指南 |

> AI Agent 开发时应优先阅读 `.agent/` 下的规则文件，遵循项目约定。
