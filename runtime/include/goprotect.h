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
 * 字符串解密（ConstObfPass 插入）
 *
 * Pass 会把可安全改写的私有字符串全局替换为加密可写全局，并生成
 * __gp_str_regions 区域表；本函数按表原位 XOR 还原（一次性，幂等守卫）。
 */
void __goprotect_decrypt_strings(void);

/**
 * 加密字符串区域表（由 ConstObfPass 生成的模块定义，运行时只读）。
 * 结构布局必须与 Go 侧 llvmwrap.EmitStrRegionsTable 的 { ptr, i32, i8 }
 * 保持一致。
 */
typedef struct goprotect_str_region {
    const uint8_t* data; /* 加密数据（模块内可写全局） */
    int32_t len;         /* 精确字节数（含 NUL） */
    uint8_t key;         /* 单字节 XOR 密钥 */
} goprotect_str_region_t;

extern const goprotect_str_region_t __gp_str_regions[];
extern const int32_t __gp_str_regions_count;

/* 钩子事件类型 */
#define GOPROTECT_HOOK_EVENT_ENTER 1
#define GOPROTECT_HOOK_EVENT_EXIT  2

typedef void (*goprotect_hook_cb)(int event);

/* 注册钩子回调（NULL 恢复默认行为） */
void goprotect_set_hook_callback(goprotect_hook_cb cb);

/**
 * VM 字节码入口（P2.3 ABI v2，统一符号）
 *
 * 编译期生成的字节码经过 opcode 随机化与 XOR 加密；入口从元数据读取
 * bytecode_len / key_pad / opcodes 解码表 / param_count，解密后把 a0..a3
 * 播种进 VM locals 再解释执行。VM 选择烘焙在字节码数据里（随机化映射与
 * key_pad），与符号无关——自定义 VM 名不会产生链接期陷阱。
 *
 * @param bytecode 加密的字节码指针（NUL 结尾的全局数组）
 * @param meta     元数据 JSON 字符串指针（NUL 结尾的全局数组）
 * @param a0..a3   虚拟化函数的实参（不足 4 个时高位为填充零）
 * @return         RET_VALUE 弹出的返回值；void 虚拟化为 0
 */
int32_t __goprotect_vm_entry_encrypted(const uint8_t* bytecode, const char* meta,
                                       int32_t a0, int32_t a1, int32_t a2, int32_t a3);

/* 历史别名（旧管线按 VM 名铸造符号），保留同签名以兼容既有产物。 */
int32_t __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode, const char* meta,
                                            int32_t a0, int32_t a1, int32_t a2, int32_t a3);
int32_t __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode, const char* meta,
                                            int32_t a0, int32_t a1, int32_t a2, int32_t a3);

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
