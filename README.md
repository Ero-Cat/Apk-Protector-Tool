# Protector Tool

<p align="center">
  <strong>LLVM-Based APK Hardening & IR Obfuscation Toolkit</strong>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/LLVM-Optional-262D3A?style=flat&logo=llvm" alt="LLVM">
  <img src="https://img.shields.io/badge/Android-NDK-3DDC84?style=flat&logo=android" alt="Android">
</p>

---

## 功能特性

| 模块 | 功能 | 说明 |
|------|------|------|
| **APK 加固** | DEX 加密 | AES-GCM + Deflate 压缩 |
| | 包名随机化 | 混淆原始包名 |
| | 伪加固标记 | 模拟主流加固特征 |
| | 自动签名 | zipalign + V1/V2 签名 |
| **IR 混淆** | 控制流平坦化 | Switch 调度器模式 |
| | 常量混淆 | 运行时解密 |
| | 指令替换 | 等价指令变换 |
| **VMP 虚拟化** | 字节码编译 | IR → 自定义 VM 指令 |
| | 多 VM 支持 | 分级保护 (normal/sensitive/critical) |
| | Opcode 随机化 | 每次构建唯一映射 |
| **安全钩子** | 完整性校验 | `__goprotect_check_integrity` |
| | 反调试 | `__goprotect_anti_debug` |

---

## 快速开始

### 环境要求

- Go 1.25+
- Android build-tools (zipalign, apksigner)
- LLVM (可选，需 `-tags llvm` 编译)

### 编译

```bash
# 基础编译
go build -o dist/protector ./cmd/protector

# 启用 LLVM 支持
go build -tags llvm -o dist/protector ./cmd/protector

# IR 混淆工具
go build -tags llvm -o dist/goprotect ./cmd/goprotect
```

### APK 加固示例

```bash
BT=~/Library/Android/sdk/build-tools/36.1.0
APK_PROTECT_SECRET="${APK_PROTECT_SECRET:?set APK_PROTECT_SECRET}"
APK_STORE_PASS="${APK_STORE_PASS:?set APK_STORE_PASS}"
APK_KEY_PASS="${APK_KEY_PASS:-$APK_STORE_PASS}"
APK_KEY_ALIAS="${APK_KEY_ALIAS:?set APK_KEY_ALIAS}"

./protector \
  -input app-release.apk \
  -output dist/app-protected.apk \
  -protect \
  -protect-random-package \
  -protect-multi-dex \
  -protect-compress \
  -protect-secret "$APK_PROTECT_SECRET" \
  -zipalign "$BT/zipalign" \
  -apksigner "$BT/apksigner" \
  -keystore sign/release.keystore \
  -store-pass "$APK_STORE_PASS" \
  -key-pass "$APK_KEY_PASS" \
  -key-alias "$APK_KEY_ALIAS" \
  -verify \
  -report dist/report.json
```

### IR 混淆示例

```bash
./goprotect \
  -config config/example.yml \
  -input module.bc \
  -o module_protected.bc
```

---

## 项目结构

```
protector-tool/
├── cmd/
│   ├── protector/       # APK 加固 CLI
│   └── goprotect/       # IR 混淆 CLI
├── config/              # 配置定义与示例
├── internal/app/        # APK 处理核心逻辑
├── passes/              # LLVM Pass 实现
│   ├── vmp/             # VMP 字节码编译器
│   ├── virtualize.go    # 虚拟化 Pass
│   ├── cf_flatten.go    # 控制流平坦化
│   ├── const_obf.go     # 常量混淆
│   └── security_hooks.go # 安全钩子
├── llvmwrap/            # LLVM C API 封装
├── runtime/             # Android 运行时库
│   ├── include/         # C 头文件
│   └── src/             # 实现源码
├── report/              # 报告生成
└── docs/                # 文档
```

---

## 配置说明

### YAML 配置示例

```yaml
input: module.bc
output: module_protected.bc

passes:
  cf_flatten: true
  const_obfuscation: true
  virtualization: true
  security_hooks: true

obfuscation:
  level: medium
  flatten_ratio: 50
  virtualize_ratio: 30

vmp:
  enable_multi_vm: true
  static_key: "${GOPROTECT_STATIC_KEY}"
  vms:
    - name: vm_a
      isa: A
    - name: vm_b
      isa: B
  levels:
    normal: vm_a
    sensitive: vm_b
    critical: vm_b
```

### CLI 参数速查

| 参数 | 说明 |
|------|------|
| `-config` | 配置文件路径 |
| `-input` | 输入 APK/BC 文件 |
| `-output` | 输出文件路径 |
| `-protect` | 启用 APK 保护 |
| `-protect-compress` | 压缩后加密 DEX |
| `-protect-random-package` | 随机化包名 |
| `-protect-multi-dex` | 加密所有 DEX |
| `-zipalign` | zipalign 路径 |
| `-apksigner` | apksigner 路径 |
| `-keystore` | 签名密钥库 |
| `-verify` | 验证签名 |
| `-report` | 报告输出路径 |

---

## 运行时集成

VMP 保护的函数需要运行时解释器支持。项目提供 `runtime/` 框架：

```c
// 完整性校验钩子
void __goprotect_check_integrity(uint32_t region_id);

// 反调试钩子
void __goprotect_anti_debug(void);

// VM 入口点
void __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode);
void __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode);
```

### 编译运行时库

```bash
cd runtime
mkdir build && cd build
cmake -DANDROID_ABI=arm64-v8a \
      -DANDROID_NDK=/path/to/ndk \
      -DCMAKE_TOOLCHAIN_FILE=$NDK/build/cmake/android.toolchain.cmake \
      ..
make
```

---

## 测试

```bash
go test ./...

# 输出示例：
# ok  protector-tool/config       5 tests
# ok  protector-tool/passes       6 tests
# ok  protector-tool/passes/vmp   5 tests
```

---

## 常见问题

| 问题 | 解决方案 |
|------|----------|
| `INSTALL_FAILED_INVALID_APK` | 确保启用 zipalign 和签名 |
| 找不到 apksigner | 安装 Android build-tools |
| APK 体积过大 | 启用 `-protect-compress` |
| Android 11+ 报错 -124 | 已修复，resources.arsc 强制不压缩 |

---

## 版本历史

| 版本 | 更新内容 |
|------|----------|
| **v1.4** | VMP 字节码编译器、控制流平坦化重写、运行时库框架、单元测试 |
| **v1.3** | 修复 Android R+ 安装问题，resources.arsc 对齐 |
| **v1.2** | 多 VM 随机化 VMP、字节码加密、分级保护 |
| **v1.1** | DEX 压缩加密、体积优化 |
| **v1.0** | 基础加固、签名、ADB 验证 |

---

## License

MIT License
