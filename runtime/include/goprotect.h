/**
 * goprotect.h - Goprotect Runtime Library Header
 *
 * 此头文件定义了 goprotect 编译期工具插入的运行时钩子接口。
 * Android 应用需要提供这些函数的实现。
 */

#ifndef GOPROTECT_H
#define GOPROTECT_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/* ============================================================================
 * 完整性检查
 * ============================================================================ */

/**
 * 完整性校验钩子
 *
 * 在编译期被插入到关键函数入口。应用应实现此函数以执行：
 * - DEX 文件哈希校验
 * - SO 库完整性检查
 * - 签名验证
 *
 * @param region_id 区域标识符，用于区分不同的检查点
 */
void __goprotect_check_integrity(uint32_t region_id);

/* ============================================================================
 * 反调试
 * ============================================================================ */

/**
 * 反调试钩子
 *
 * 在编译期被插入到关键函数入口。应用应实现此函数以检测：
 * - 调试器附加 (TracerPid)
 * - Frida 注入
 * - Xposed 框架
 * - 模拟器环境
 */
void __goprotect_anti_debug(void);

/* ============================================================================
 * VM 入口点
 * ============================================================================ */

/**
 * VM 字节码入口（加密版本）
 *
 * 每个 VM 有一个对应入口。编译期生成的字节码经过 XOR 加密，
 * 运行时需先解密再执行。
 *
 * @param bytecode 加密的字节码指针
 */
void __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode);
void __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode);

/* ============================================================================
 * 字节码元数据（JSON 格式，嵌入二进制）
 * ============================================================================
 *
 * 每个虚拟化函数会生成两个全局变量：
 * - __gp_bc_<fn>      : 加密后的字节码
 * - __gp_bc_meta_<fn> : JSON 元数据
 *
 * 元数据格式:
 * {
 *   "fn": "<函数名>",
 *   "vm": "<VM名称>",
 *   "build_nonce": <构建随机数>,
 *   "key_hint": "<运行时密钥提示>",
 *   "bytecode_len": <字节码长度>,
 *   "local_count": <局部变量数>,
 *   "ext_funcs": {...}
 * }
 */

/* ============================================================================
 * 辅助宏
 * ============================================================================ */

#ifdef GOPROTECT_IMPLEMENTATION

/* 示例：简单的 XOR 解密 */
static inline void gp_decrypt_xor(uint8_t* data, size_t len, uint8_t key) {
    for (size_t i = 0; i < len; i++) {
        data[i] ^= key;
    }
}

#endif /* GOPROTECT_IMPLEMENTATION */

#ifdef __cplusplus
}
#endif

#endif /* GOPROTECT_H */
