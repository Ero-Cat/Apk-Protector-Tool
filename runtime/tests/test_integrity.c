/**
 * test_integrity.c - 完整性校验真实化测试（P3.1）
 *
 * 覆盖：注册基线 / 校验通过 / 篡改检出 / 失败计数 / 未注册区域放行 /
 * ZEROIZE 策略效果。
 *
 * 编译运行：
 *   clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
 *     runtime/tests/test_integrity.c -o /tmp/test_integrity && /tmp/test_integrity
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../src/integrity.c"

static int g_failures = 0;

#define CHECK(cond)                                                     \
    do {                                                                \
        if (!(cond)) {                                                  \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond); \
            g_failures++;                                               \
        }                                                               \
    } while (0)

int main(void) {
    unsigned char buf[64];
    memset(buf, 'A', sizeof buf);

    /* 1. 注册基线：未改动时校验通过。 */
    goprotect_register_region(7, buf, sizeof buf);
    CHECK(goprotect_verify_region(7) == 1);
    CHECK(goprotect_has_integrity_failures() == 0);

    /* 2. 篡改一个字节：必须检出且计数增长（默认 LOG 策略）。 */
    buf[13] ^= 0x5A;
    CHECK(goprotect_verify_region(7) == 0);
    CHECK(goprotect_has_integrity_failures() == 1);

    /* 3. 还原后恢复通过（失败计数保留，只增不减）。 */
    buf[13] ^= 0x5A;
    CHECK(goprotect_verify_region(7) == 1);
    int checks = 0, fails = 0;
    goprotect_get_integrity_stats(&checks, &fails);
    CHECK(fails == 1);

    /* 4. 未注册区域：返回 0 但不计入失败（无法校验 != 校验失败）。 */
    CHECK(goprotect_verify_region(999) == 0);
    goprotect_get_integrity_stats(&checks, &fails);
    CHECK(fails == 1);

    /* 5. ZEROIZE 策略：篡改后区域被擦除。 */
    unsigned char zero_buf[16];
    memset(zero_buf, 0xEE, sizeof zero_buf);
    goprotect_register_region(8, zero_buf, sizeof zero_buf);
    goprotect_set_integrity_policy(GOPROTECT_INTEGRITY_POLICY_ZEROIZE);
    zero_buf[0] = 0x11; /* 篡改 */
    CHECK(goprotect_verify_region(8) == 0);
    int zeroed = 1;
    for (size_t i = 0; i < sizeof zero_buf; i++) {
        if (zero_buf[i] != 0) zeroed = 0;
    }
    CHECK(zeroed);
    goprotect_set_integrity_policy(GOPROTECT_INTEGRITY_POLICY_LOG);

    if (g_failures == 0) {
        printf("test_integrity: all checks passed\n");
        return 0;
    }
    printf("test_integrity: %d check(s) failed\n", g_failures);
    return 1;
}
