/**
 * aes_gcm.c - 最小 AES-256-GCM 实现（解密方向）
 *
 * 组成：AES-256 单块加密 + GF(2^128) GHASH + CTR 模式。刻意只实现加载器
 * 需要的解密路径；密钥扩展与加密块遵循 FIPS-197，GCM 遵循 NIST SP 800-38D。
 */

#include "aes_gcm.h"
#include <string.h>

#define AES_256_ROUNDS 14
#define AES_BLOCK 16

/* Rijndael S-box */
static const uint8_t sbox[256] = {
    0x63,0x7c,0x77,0x7b,0xf2,0x6b,0x6f,0xc5,0x30,0x01,0x67,0x2b,0xfe,0xd7,0xab,0x76,
    0xca,0x82,0xc9,0x7d,0xfa,0x59,0x47,0xf0,0xad,0xd4,0xa2,0xaf,0x9c,0xa4,0x72,0xc0,
    0xb7,0xfd,0x93,0x26,0x36,0x3f,0xf7,0xcc,0x34,0xa5,0xe5,0xf1,0x71,0xd8,0x31,0x15,
    0x04,0xc7,0x23,0xc3,0x18,0x96,0x05,0x9a,0x07,0x12,0x80,0xe2,0xeb,0x27,0xb2,0x75,
    0x09,0x83,0x2c,0x1a,0x1b,0x6e,0x5a,0xa0,0x52,0x3b,0xd6,0xb3,0x29,0xe3,0x2f,0x84,
    0x53,0xd1,0x00,0xed,0x20,0xfc,0xb1,0x5b,0x6a,0xcb,0xbe,0x39,0x4a,0x4c,0x58,0xcf,
    0xd0,0xef,0xaa,0xfb,0x43,0x4d,0x33,0x85,0x45,0xf9,0x02,0x7f,0x50,0x3c,0x9f,0xa8,
    0x51,0xa3,0x40,0x8f,0x92,0x9d,0x38,0xf5,0xbc,0xb6,0xda,0x21,0x10,0xff,0xf3,0xd2,
    0xcd,0x0c,0x13,0xec,0x5f,0x97,0x44,0x17,0xc4,0xa7,0x7e,0x3d,0x64,0x5d,0x19,0x73,
    0x60,0x81,0x4f,0xdc,0x22,0x2a,0x90,0x88,0x46,0xee,0xb8,0x14,0xde,0x5e,0x0b,0xdb,
    0xe0,0x32,0x3a,0x0a,0x49,0x06,0x24,0x5c,0xc2,0xd3,0xac,0x62,0x91,0x95,0xe4,0x79,
    0xe7,0xc8,0x37,0x6d,0x8d,0xd5,0x4e,0xa9,0x6c,0x56,0xf4,0xea,0x65,0x7a,0xae,0x08,
    0xba,0x78,0x25,0x2e,0x1c,0xa6,0xb4,0xc6,0xe8,0xdd,0x74,0x1f,0x4b,0xbd,0x8b,0x8a,
    0x70,0x3e,0xb5,0x66,0x48,0x03,0xf6,0x0e,0x61,0x35,0x57,0xb9,0x86,0xc1,0x1d,0x9e,
    0xe1,0xf8,0x98,0x11,0x69,0xd9,0x8e,0x94,0x9b,0x1e,0x87,0xe9,0xce,0x55,0x28,0xdf,
    0x8c,0xa1,0x89,0x0d,0xbf,0xe6,0x42,0x68,0x41,0x99,0x2d,0x0f,0xb0,0x54,0xbb,0x16
};

static const uint8_t rcon[11] = {
    0x00, 0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80, 0x1b, 0x36
};

/* AES-256 轮密钥（60 个 32 位字）。 */
typedef struct {
    uint32_t rk[4 * (AES_256_ROUNDS + 1)];
} aes256_ctx;

static uint32_t ld32(const uint8_t* p) {
    return ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) |
           ((uint32_t)p[2] << 8) | (uint32_t)p[3];
}

static void st32(uint8_t* p, uint32_t v) {
    p[0] = (uint8_t)(v >> 24);
    p[1] = (uint8_t)(v >> 16);
    p[2] = (uint8_t)(v >> 8);
    p[3] = (uint8_t)v;
}

static void aes256_expand(aes256_ctx* ctx, const uint8_t key[32]) {
    static const uint32_t NK = 8; /* AES-256 */
    uint32_t* w = ctx->rk;
    for (uint32_t i = 0; i < NK; i++) {
        w[i] = ld32(key + 4 * i);
    }
    for (uint32_t i = NK; i < 4 * (AES_256_ROUNDS + 1); i++) {
        uint32_t t = w[i - 1];
        if (i % NK == 0) {
            /* RotWord + SubWord + Rcon */
            t = (t << 8) | (t >> 24);
            t = ((uint32_t)sbox[(t >> 24) & 0xFF] << 24) |
                ((uint32_t)sbox[(t >> 16) & 0xFF] << 16) |
                ((uint32_t)sbox[(t >> 8) & 0xFF] << 8) |
                (uint32_t)sbox[t & 0xFF];
            t ^= (uint32_t)rcon[i / NK] << 24;
        } else if (i % NK == 4) {
            t = ((uint32_t)sbox[(t >> 24) & 0xFF] << 24) |
                ((uint32_t)sbox[(t >> 16) & 0xFF] << 16) |
                ((uint32_t)sbox[(t >> 8) & 0xFF] << 8) |
                (uint32_t)sbox[t & 0xFF];
        }
        w[i] = w[i - NK] ^ t;
    }
}

static void add_round_key(uint8_t s[16], const uint32_t* rk) {
    for (int i = 0; i < 4; i++) {
        uint8_t tmp[4];
        st32(tmp, rk[i]);
        for (int j = 0; j < 4; j++) {
            s[4 * i + j] ^= tmp[j];
        }
    }
}

static void sub_bytes(uint8_t s[16]) {
    for (int i = 0; i < 16; i++) s[i] = sbox[s[i]];
}

static void shift_rows(uint8_t s[16]) {
    uint8_t t;
    /* row 1 */
    t = s[1]; s[1] = s[5]; s[5] = s[9]; s[9] = s[13]; s[13] = t;
    /* row 2 */
    t = s[2]; s[2] = s[10]; s[10] = t;
    t = s[6]; s[6] = s[14]; s[14] = t;
    /* row 3 */
    t = s[15]; s[15] = s[11]; s[11] = s[7]; s[7] = s[3]; s[3] = t;
}

static uint8_t xtime(uint8_t x) {
    return (uint8_t)((x << 1) ^ ((x & 0x80) ? 0x1B : 0x00));
}

static uint8_t mul2(uint8_t x) { return xtime(x); }
static uint8_t mul3(uint8_t x) { return (uint8_t)(xtime(x) ^ x); }

static void mix_columns(uint8_t s[16]) {
    for (int c = 0; c < 4; c++) {
        uint8_t* p = s + 4 * c;
        uint8_t a0 = p[0], a1 = p[1], a2 = p[2], a3 = p[3];
        p[0] = (uint8_t)(mul2(a0) ^ mul3(a1) ^ a2 ^ a3);
        p[1] = (uint8_t)(a0 ^ mul2(a1) ^ mul3(a2) ^ a3);
        p[2] = (uint8_t)(a0 ^ a1 ^ mul2(a2) ^ mul3(a3));
        p[3] = (uint8_t)(mul3(a0) ^ a1 ^ a2 ^ mul2(a3));
    }
}

static void aes256_encrypt_block(const aes256_ctx* ctx, const uint8_t in[16], uint8_t out[16]) {
    uint8_t s[16];
    memcpy(s, in, 16);

    add_round_key(s, ctx->rk);
    for (int r = 1; r < AES_256_ROUNDS; r++) {
        sub_bytes(s);
        shift_rows(s);
        mix_columns(s);
        add_round_key(s, ctx->rk + 4 * r);
    }
    sub_bytes(s);
    shift_rows(s);
    add_round_key(s, ctx->rk + 4 * AES_256_ROUNDS);

    memcpy(out, s, 16);
}

/* GF(2^128) 乘法（GHASH 核心，右移规范化表示）。 */
static void gf128_mul(uint8_t block[16], const uint8_t h[16]) {
    uint8_t v[16];
    uint8_t z[16];
    memcpy(v, h, 16);
    memset(z, 0, 16);

    for (int i = 0; i < 128; i++) {
        int bit = (block[i / 8] >> (7 - (i % 8))) & 1;
        if (bit) {
            for (int j = 0; j < 16; j++) z[j] ^= v[j];
        }
        int lsb = v[15] & 1;
        for (int j = 15; j > 0; j--) {
            v[j] = (uint8_t)((v[j] >> 1) | (uint8_t)(v[j - 1] << 7));
        }
        v[0] >>= 1;
        if (lsb) v[0] ^= 0xE1;
    }
    memcpy(block, z, 16);
}

static void inc32(uint8_t block[16]) {
    for (int i = 15; i >= 12; i--) {
        if (++block[i] != 0) break;
    }
}

int gp_aes256gcm_decrypt(const uint8_t* key,
                         const uint8_t* nonce,
                         const uint8_t* ct, size_t ct_len,
                         uint8_t* out) {
    if (ct_len < GP_GCM_TAG_LEN) {
        return -1;
    }
    size_t body = ct_len - GP_GCM_TAG_LEN;
    const uint8_t* tag = ct + body;

    aes256_ctx ctx;
    aes256_expand(&ctx, key);

    uint8_t h[16], j0[16], ej0[16];
    memset(h, 0, 16);
    aes256_encrypt_block(&ctx, h, h); /* H = E(K, 0^128) */

    /* J0 = IV || 0^31 || 1（96-bit nonce 路径） */
    memset(j0, 0, 16);
    memcpy(j0, nonce, GP_GCM_NONCE_LEN);
    j0[15] = 0x01;
    aes256_encrypt_block(&ctx, j0, ej0); /* E(K, J0)，用于 tag 掩码 */

    /* GHASH over the ciphertext（无 AAD）。 */
    uint8_t g[16];
    memset(g, 0, 16);
    for (size_t off = 0; off < body; off += 16) {
        uint8_t blk[16];
        size_t n = body - off;
        if (n > 16) n = 16;
        memset(blk, 0, 16);
        memcpy(blk, ct + off, n);
        for (int i = 0; i < 16; i++) g[i] ^= blk[i];
        gf128_mul(g, h);
    }
    /* 末块：||len(C)||_64（无 AAD，低 64 位为密文位长）。 */
    uint8_t lenblk[16];
    memset(lenblk, 0, 16);
    uint64_t bits = (uint64_t)body * 8;
    for (int i = 0; i < 8; i++) {
        lenblk[15 - i] = (uint8_t)(bits >> (8 * i));
    }
    for (int i = 0; i < 16; i++) g[i] ^= lenblk[i];
    gf128_mul(g, h);

    /* T = E(K, J0) ^ GHASH(C)。 */
    uint8_t calc[16];
    for (int i = 0; i < 16; i++) {
        calc[i] = ej0[i] ^ g[i];
    }
    /* 常数时间比较。 */
    uint8_t diff = 0;
    for (int i = 0; i < 16; i++) {
        diff |= (uint8_t)(calc[i] ^ tag[i]);
    }
    if (diff != 0) {
        return -1;
    }

    /* CTR 解密（计数器从 inc32(J0) 起步）。 */
    uint8_t ctr[16];
    memcpy(ctr, j0, 16);
    for (size_t off = 0; off < body; off += 16) {
        inc32(ctr);
        uint8_t ks[16];
        aes256_encrypt_block(&ctx, ctr, ks);
        size_t n = body - off;
        if (n > 16) n = 16;
        for (size_t i = 0; i < n; i++) {
            out[off + i] = ct[off + i] ^ ks[i];
        }
    }
    return (int)body;
}
