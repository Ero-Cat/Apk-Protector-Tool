# ADR-0001: DEX 加密密钥的交付方式

- 状态：已接受（2026-10）
- 关联：ROADMAP P3.3（🔴 安全）

## 背景与问题

protector 对 `classes*.dex` 做 AES-256-GCM 加密后，把 **base64 密钥写进 APK 内的
`assets/protector/metadata.json`**——密钥与密文同体分发，攻击者解包即得密钥，
加密对抗静态分析的意义为零。同时仓库不含任何 Android 侧加载/解密组件，
加固产物开箱无法运行。

## 候选方案

| 方案 | 思路 | 取舍 |
|------|------|------|
| A. 密钥外置文件 | 密钥只落构建侧 `<output>.key`（0600），由发布流程注入加载器 | 简单、彻底消除"同体分发"；需要发布链路配合 |
| B. NDK 编译期注入 + 拆分混淆 | 密钥拆成多段常量编译进 native 库 | 提高静态提取成本；本质仍是混淆，可被逆向 |
| C. JNI 运行时拼装 + 签名绑定 | 密钥材料分散在 Java/native，运行时校验 APK 签名后拼装 | 增加动态分析成本；签名校验本身可被 hook |
| D. 白盒密码 | 密钥白盒化进查找表 | 工程量大、表体积大，属于商业加固层级 |

## 决策

**采用 A（密钥外置）作为基线，B 的"拆分常量拼装"作为加载器内部的实现细节。**

理由：
1. 安全边界先讲清楚：**任何随 APK 分发的密钥材料最终都可被提取**（B/C/D 只是
   提高成本）。把密钥移出 APK 是唯一能兑现"APK 内不再含可直接使用的密钥材料"
   这条验收标准的结构性改动。
2. A 不预设应用架构：demo 加载器（`runtime/android/`）演示"拆分常量拼装 +
   AES-GCM 解密 + `InMemoryDexClassLoader`"，应用可替换成自己的密钥注入通道
   （服务端下发、签名派生、硬件密钥……）。
3. 保留 `protections.embed_key` 作为 legacy 逃生开关（默认关，文档标注不安全），
   兼容旧的"外部分析脚本从 metadata 取密钥"流程。

## 后果

- protector：`metadata.json` 拆为**运行时元数据**（算法/nonce/长度/映射，留在
  APK 内）与**构建期密钥**（`<final_output>.key`，0600，绝不入包）两份。
- 加载器 demo：`runtime/android/dex_decrypt.c`（host 可编译可测的核心）、
  `crypto/aes_gcm.c`（AES-256-GCM，NIST 向量 + Go 交叉向量双锚定）、
  `goprotect_loader.c`（JNI 胶水，Android-only）、Java 侧
  `InMemoryDexClassLoader` 示例。
- 真机验收（demo App 启动）为文档化手动步骤；host 侧以"产物无密钥材料 +
  共享向量解密还原原始 dex 字节"等价覆盖。
- 白盒密码与签名绑定派生列为后续独立条目，不与本 ADR 混淆。
