# runtime/android — NDK demo 加载器（P3.3）

配合 protector 的密钥外置方案（[ADR-0001](../../docs/design/adr-0001-dex-key-delivery.md)），
演示"native 拿密钥 → AES-GCM 解密 → `InMemoryDexClassLoader` 装载"的最小链路。

## 组成

| 文件 | 说明 | host 可测 |
|------|------|-----------|
| `crypto/aes_gcm.{h,c}` | 最小 AES-256-GCM（解密+验签），与 Go `crypto/aes`+`cipher.NewGCM` 产物格式一致 | ✅ NIST + Go 交叉向量 |
| `dex_decrypt.{h,c}` | 解密核心：拆分密钥异或拼装 + payload 解密 | ✅ `tests/test_dex_decrypt.c` |
| `goprotect_loader.c` | JNI 胶水（读 assets → 拼密钥 → 解密 → 回传字节），`__ANDROID__` 门后 | ❌ 需 NDK |
| `java/.../GoprotectLoader.java` | `Application` 子类示例（`InMemoryDexClassLoader`） | — |
| `CMakeLists.txt` | NDK 构建 | — |

## 使用步骤

1. protector 侧（默认配置即外置密钥）：

   ```bash
   protector run -profile full -input app-release.apk \
     -protect-secret-env APK_PROTECT_SECRET ...
   # 产物：dist/app-protected.apk + dist/app-protected.apk.key（0600）
   ```

   ⚠️ demo 约定 `compress_before_encrypt: false`（Deflate 需在加载器接 zlib）。

2. 把 `<output>.key` 中的 `aes_key`（base64 解码 32 字节）拆成两片异或分片，
   写入 `goprotect_loader.c` 的 `kKeyPart0/kKeyPart1`（发布流程自动生成，
   不要手写完整密钥）。

3. App 工程接入：
   - `settings.gradle` 里 include 本目录（或复制 CMakeLists 到工程）；
   - `AndroidManifest.xml` 的 `application` 节点指向 `com.goprotect.demo.GoprotectLoader`
     （或在自己的 Application 里调用 `GoprotectLoader.loadProtectedDex(ctx)`）；
   - 首个入口类经返回的 ClassLoader 反射加载。

4. NDK 构建：

   ```bash
   cmake -DCMAKE_TOOLCHAIN_FILE=$NDK/build/cmake/android.toolchain.cmake \
         -DANDROID_ABI=arm64-v8a -B build && cmake --build build
   ```

## 真机手动验收（文档化步骤）

host 侧等价验证已由 CI 覆盖（产物无密钥材料 + 共享向量解密还原）；CI 同时
对 `runtime/src` 与本目录的可移植核心跑 **aarch64 交叉编译冒烟**
（`gcc-aarch64-linux-gnu`，Android 主目标架构），捕捉 socket/格式串/头文件
类回归。真机验收：

1. 构造 demo App（含一个入口 Activity），用 protector 加固（密钥外置默认开启）；
2. 按上述步骤注入分片密钥、打 NDK 包、安装到设备；
3. 启动 App：应正常进入入口 Activity，`adb logcat | grep GoprotectLoader`
   出现 `protected dex loaded in memory`；
4. `adb shell pm path <pkg>` 解包产物，确认 `assets/protector/metadata.json`
   无 `encryption_key` 字段；
5. 反向用例：改动任一密钥分片一位 → App 启动解密失败（验签不过），
   logcat 出现 `decrypt failed`。

## 安全边界（务必阅读）

- 拆分常量只是**提高静态提取成本**的混淆；随包分发的任何密钥材料最终都可被
  逆向提取。生产环境请把密钥注入通道替换为服务端下发 / 签名派生 / 硬件密钥。
- 白盒密码与签名绑定派生是后续独立工作（见 ROADMAP P3 后续条目）。
