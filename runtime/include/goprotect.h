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
 * 入口/出口钩子（EntryExitPass 插入）
 *
 * 默认实现见 runtime/src/hooks.c；应用可注册回调接管。
 */
void __goprotect_hook(void);

/**
 * 字符串解密存根（ConstObfPass 插入）
 */
void __goprotect_decrypt_strings(void);

/* 钩子事件类型 */
#define GOPROTECT_HOOK_EVENT_ENTER 1
#define GOPROTECT_HOOK_EVENT_EXIT  2

typedef void (*goprotect_hook_cb)(int event);

/* 注册钩子回调（NULL 恢复默认行为） */
void goprotect_set_hook_callback(goprotect_hook_cb cb);

/**
 * VM 字节码入口（加密版本）
 *
 * 每个 VM 有一个对应入口。编译期生成的字节码经过 opcode 随机化与 XOR
 * 加密；运行时从元数据读取 bytecode_len / key_pad / opcodes 解码表，
 * 解密后解释执行。
 *
 * @param bytecode 加密的字节码指针（NUL 结尾的全局数组）
 * @param meta     元数据 JSON 字符串指针（NUL 结尾的全局数组）
 */
void __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode, const char* meta);
void __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode, const char* meta);

/**
 * 注册静态密钥片段（与编译期 vmp.static_key 的末字节一致，默认 0x5A）。
 * 最终解密密钥 = 注册的静态片段 ^ 元数据 key_pad。
 */
void goprotect_set_static_key(uint8_t key);

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
 *   "bytecode_len": <字节码长度>,
 *   "local_count": <局部变量数>,
 *   "param_count": <参数数量>,
 *   "encrypted": true,
 *   "key_pad": <随机密钥片段>,
 *   "key_hint": "<运行时密钥提示>",
 *   "opcodes": { "ADD": <随机化字节>, ... },
 *   "ext_funcs": { "<符号名>": <函数ID>, ... }
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
