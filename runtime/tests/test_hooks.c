/**
 * test_hooks.c - __goprotect_decrypt_strings 区域表解密往返测试（P2.2）
 *
 * 编译运行：
 *   clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
 *     runtime/tests/test_hooks.c -o /tmp/test_hooks && /tmp/test_hooks
 */
#include <stdio.h>
#include <string.h>

#include "../src/hooks.c" /* brings in the decrypt implementation */

/* 模拟 Go 侧 EmitStrRegionsTable 产出的表：两个密文全局 + 一个空占位槽。
 * 密文 = 明文 XOR key（与 pass 侧逐字节一致）。 */
static uint8_t g_blob1[] = { /* "secret!" ^ 0x5A */
    0x29, 0x3F, 0x39, 0x28, 0x3F, 0x2E, 0x7B, 0x5A,
};
static uint8_t g_blob2[] = { /* "decode" ^ 0x21 */
    0x45, 0x44, 0x42, 0x4E, 0x45, 0x44, 0x21,
};

const goprotect_str_region_t __gp_str_regions[] = {
    { g_blob1, (int32_t)sizeof g_blob1, 0x5A },
    { g_blob2, (int32_t)sizeof g_blob2, 0x21 },
    { NULL, 0, 0 }, /* 空占位槽必须被安全跳过 */
};
const int32_t __gp_str_regions_count = 3;

static int g_failures = 0;

#define CHECK(cond)                                     \
    do {                                                \
        if (!(cond)) {                                  \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond); \
            g_failures++;                               \
        }                                               \
    } while (0)

int main(void) {
    __goprotect_decrypt_strings();

    CHECK(strcmp((const char*)g_blob1, "secret!") == 0);
    CHECK(strcmp((const char*)g_blob2, "decode") == 0);

    /* 幂等：重复调用不得把数据再加密回去。 */
    __goprotect_decrypt_strings();
    CHECK(strcmp((const char*)g_blob1, "secret!") == 0);
    CHECK(strcmp((const char*)g_blob2, "decode") == 0);

    if (g_failures == 0) {
        printf("test_hooks: all checks passed\n");
        return 0;
    }
    printf("test_hooks: %d check(s) failed\n", g_failures);
    return 1;
}
