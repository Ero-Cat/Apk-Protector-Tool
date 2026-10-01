/**
 * aes_gcm.h - 最小 AES-256-GCM（解密 + 验签）
 *
 * 仅为 demo 加载器服务：密钥 32 字节、nonce 12 字节、tag 16 字节（与
 * protector 的 Go crypto/aes + cipher.NewGCM 产物格式一致：密文后追加 tag）。
 * 正确性由 NIST GCM 向量与 Go 交叉向量双锚定（见 runtime/android/tests/）。
 */

#ifndef GOPROTECT_AES_GCM_H
#define GOPROTECT_AES_GCM_H

#include <stddef.h>
#include <stdint.h>

#define GP_GCM_KEY_LEN 32
#define GP_GCM_NONCE_LEN 12
#define GP_GCM_TAG_LEN 16

/* 解密并验证 tag。成功返回明文长度（= ct_len - 16），失败返回 -1。
 * out 需要至少 ct_len - 16 字节容量；out 与 ct 可原地重叠。 */
int gp_aes256gcm_decrypt(const uint8_t* key,
                         const uint8_t* nonce,
                         const uint8_t* ct, size_t ct_len,
                         uint8_t* out);

#endif /* GOPROTECT_AES_GCM_H */
