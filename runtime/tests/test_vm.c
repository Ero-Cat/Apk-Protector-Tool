/**
 * test_vm.c - VM 解释器单元测试
 *
 * 直接包含 vm_entry.c（单翻译区测试），验证：
 *   1. 元数据 JSON 解析 + opcode 解码表构建
 *   2. XOR 解密（static ^ key_pad）
 *   3. STORE_LOCAL/PUSH_LOCAL 槽位值模型
 *   4. 算术与 CALL_EXT 外部函数调用
 *   5. 缺失 bytecode_len 时的安全拒绝
 *
 * 构建: clang -std=c11 -I ../include test_vm.c -o test_vm && ./test_vm
 */

#include "../src/vm_entry.c"
#include <assert.h>
#include <stdio.h>
#include <string.h>

static int32_t g_captured;
static int g_ext_calls;

static void test_ext(vm_context_t* ctx, int argc) {
    (void)argc;
    g_ext_calls++;
    g_captured = VM_PEEK(ctx);
}

/* 便捷编码：按标准 opcode 生成一段程序并随机化 + 加密。 */
typedef struct {
    uint8_t bytes[64];
    size_t len;
    char meta[512];
} test_program_t;

static uint8_t key_of(uint8_t pad) { return (uint8_t)(0x5A ^ pad); }

/* 构建测试程序：stmts 为 (标准opcode, 操作数...) 序列由调用方手工拼装。 */
static void build_program(test_program_t* tp, const uint8_t* plain, size_t n,
                          const char* pairs, uint8_t pad) {
    uint8_t key = key_of(pad);
    for (size_t i = 0; i < n; i++) {
        tp->bytes[i] = (uint8_t)(plain[i] ^ key);
    }
    tp->len = n;
    snprintf(tp->meta, sizeof tp->meta,
             "{\"bytecode_len\":%zu,\"encrypted\":true,\"key_pad\":%u,"
             "\"local_count\":8,\"param_count\":0,\"opcodes\":{%s}}",
             n, (unsigned)pad, pairs);
}

static const char* kPairs =
    "\"PUSH_CONST\":17,\"STORE_LOCAL\":34,\"PUSH_LOCAL\":51,"
    "\"ADD\":16,\"CALL_EXT\":68,\"RET\":85,\"JMP\":86,\"CMP_LT\":4";

void test_slot_value_model(void) {
    test_program_t tp;
    /* PUSH_CONST 42; STORE_LOCAL 3; PUSH_LOCAL 3; CALL_EXT 0 0; RET */
    uint8_t plain[] = {
        0x01, 42, 0, 0, 0,   /* PUSH_CONST 42 (LE) */
        0x32, 3,             /* STORE_LOCAL slot 3 */
        0x03, 3,             /* PUSH_LOCAL slot 3 */
        0x60, 0, 0, 0,       /* CALL_EXT fn 0 argc 0 */
        0xFF,                /* RET */
    };
    build_program(&tp, plain, sizeof plain, kPairs, /*pad=*/0x21);

    g_captured = -1;
    g_ext_calls = 0;
    assert(goprotect_register_ext_func(0, test_ext) == 0);
    __goprotect_vm_entry_encrypted_vm_a(tp.bytes, tp.meta);

    assert(g_ext_calls == 1);
    assert(g_captured == 42);
    printf("test_slot_value_model: PASS (slot roundtrip = %d)\n", g_captured);
}

void test_arithmetic(void) {
    test_program_t tp;
    /* PUSH_CONST 3; PUSH_CONST 4; ADD; STORE_LOCAL 5; PUSH_LOCAL 5; CALL_EXT; RET */
    uint8_t plain[] = {
        0x01, 3, 0, 0, 0,
        0x01, 4, 0, 0, 0,
        0x10,                /* ADD */
        0x32, 5,             /* STORE_LOCAL 5 */
        0x03, 5,             /* PUSH_LOCAL 5 */
        0x60, 0, 0, 0,
        0xFF,
    };
    build_program(&tp, plain, sizeof plain, kPairs, /*pad=*/0x63);

    g_captured = -1;
    g_ext_calls = 0;
    __goprotect_vm_entry_encrypted_vm_b(tp.bytes, tp.meta);

    assert(g_ext_calls == 1);
    assert(g_captured == 7);
    printf("test_arithmetic: PASS (3 + 4 = %d)\n", g_captured);
}

void test_missing_meta_refuses(void) {
    test_program_t tp;
    uint8_t plain[] = {0xFF};
    build_program(&tp, plain, sizeof plain, kPairs, 0x00);
    /* 抹掉 bytecode_len 字段 */
    char* p = strstr(tp.meta, "\"bytecode_len\"");
    assert(p != NULL);
    memcpy(p, "\"Xytecode_len\"", 14);

    g_ext_calls = 0;
    __goprotect_vm_entry_encrypted_vm_a(tp.bytes, tp.meta);
    assert(g_ext_calls == 0); /* 必须拒绝执行 */
    printf("test_missing_meta_refuses: PASS\n");
}

void test_identity_fallback(void) {
    /* 无 opcodes 字段：解码表恒等，字节码用标准值，明文不加密。 */
    uint8_t code[] = {0x01, 9, 0, 0, 0, 0x60, 0, 0, 0, 0xFF};
    char meta[128];
    snprintf(meta, sizeof meta, "{\"bytecode_len\":%zu,\"encrypted\":false}", sizeof code);
    g_captured = -1;
    g_ext_calls = 0;
    __goprotect_vm_entry_encrypted_vm_a(code, meta);
    assert(g_captured == 9);
    printf("test_identity_fallback: PASS (unencrypted, standard opcodes)\n");
}

int main(void) {
    test_slot_value_model();
    test_arithmetic();
    test_missing_meta_refuses();
    test_identity_fallback();
    printf("all vm tests passed\n");
    return 0;
}
