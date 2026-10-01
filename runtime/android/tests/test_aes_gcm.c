/**
 * test_aes_gcm.c - AES-256-GCM 正确性双锚定测试
 *
 *   1. NIST GCM 测试向量（Test Case 14：256-bit key）；
 *   2. Go crypto/aes + cipher.NewGCM 交叉向量（与 protector 加密路径同源，
 *      由仓库内 Go 程序生成后固化于此）。
 *
 * 编译运行：
 *   clang -std=c11 -Wall -Wextra -Werror -I runtime/android \
 *     runtime/android/tests/test_aes_gcm.c runtime/android/crypto/aes_gcm.c \
 *     -o /tmp/test_aes_gcm && /tmp/test_aes_gcm
 */
#include <stdio.h>
#include <string.h>

#include "crypto/aes_gcm.h"

static int g_failures = 0;

#define CHECK(cond)                                                     \
    do {                                                                \
        if (!(cond)) {                                                  \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond); \
            g_failures++;                                               \
        }                                                               \
    } while (0)

/* NIST GCM Test Case 14: zero key/IV/PT (AES-256)。 */
static void test_nist_vector(void) {
    uint8_t key[32] = {0};
    uint8_t nonce[12] = {0};
    /* 密文 16B + tag 16B */
    uint8_t ct_tag[32] = {
        0xce,0xa7,0x40,0x3d,0x4d,0x60,0x6b,0x6e,0x07,0x4e,0xc5,0xd3,0xba,0xf3,0x9d,0x18,
        0xd0,0xd1,0xc8,0xa7,0x99,0x99,0x6b,0xf0,0x26,0x5b,0x98,0xb5,0xd4,0x8a,0xb9,0x19
    };
    uint8_t out[16];
    int n = gp_aes256gcm_decrypt(key, nonce, ct_tag, sizeof ct_tag, out);
    CHECK(n == 16);
    for (int i = 0; i < 16; i++) {
        CHECK(out[i] == 0);
    }
    printf("test_nist_vector: PASS\n");
}

/* Go 交叉向量：key=sha256("goprotect-demo-secret")，明文为代表性 dex 头。 */
static void test_go_cross_vector(void) {
    static const uint8_t key[32] = {
        0x52,0x47,0x5e,0x1b,0xd7,0x0b,0x39,0x3a,0x51,0x43,0x44,0xa5,0x56,0x1c,0x84,0x38,
        0x53,0x6c,0xf2,0xa8,0x69,0x6c,0xf0,0xb8,0x8a,0x51,0x23,0x40,0x7b,0x85,0x66,0x60
    };
    static const uint8_t nonce[12] = {
        0x0a,0x0b,0x0c,0x01,0x02,0x03,0x04,0x05,0x06,0x07,0x08,0x09
    };
    static const uint8_t plain[41] = {
        'd','e','x','\n','0','3','5',0,
        'g','o','p','r','o','t','e','c','t',' ','d','e','m','o',' ',
        'p','a','y','l','o','a','d',' ','0','1','2','3','4','5','6','7','8','9'
    };
    static const uint8_t ct_tag[57] = {
        0xaa,0x7c,0x60,0xc9,0xa0,0x58,0x2e,0x07,0x4b,0x59,0x4e,0x4f,0x41,0x56,0x23,0x42,
        0x72,0xed,0xb4,0xf8,0x3e,0x78,0xf2,0x47,0x08,0x91,0xd2,0x93,0x6d,0xe5,0x6f,0x0c,
        0x78,0xbe,0x83,0x88,0xe2,0x21,0x91,0x81,0x21,0x91,0x4a,0x05,0xe8,0x9e,0xe6,0x0b,
        0x6e,0x26,0x81,0xf0,0xd6,0x36,0x44,0x61,0xe4
    };

    uint8_t out[64];
    int n = gp_aes256gcm_decrypt(key, nonce, ct_tag, sizeof ct_tag, out);
    CHECK(n == 41);
    CHECK(n <= 0 || memcmp(out, plain, 41) == 0);

    /* 篡改一个密文字节必须验签失败。 */
    uint8_t broken[57];
    memcpy(broken, ct_tag, sizeof broken);
    broken[3] ^= 0x40;
    CHECK(gp_aes256gcm_decrypt(key, nonce, broken, sizeof broken, out) == -1);

    /* 密钥错误必须验签失败。 */
    uint8_t badkey[32];
    memcpy(badkey, key, 32);
    badkey[0] ^= 1;
    CHECK(gp_aes256gcm_decrypt(badkey, nonce, ct_tag, sizeof ct_tag, out) == -1);

    printf("test_go_cross_vector: PASS\n");
}

int main(void) {
    test_nist_vector();
    test_go_cross_vector();
    if (g_failures == 0) {
        printf("test_aes_gcm: all checks passed\n");
        return 0;
    }
    printf("test_aes_gcm: %d check(s) failed\n", g_failures);
    return 1;
}
