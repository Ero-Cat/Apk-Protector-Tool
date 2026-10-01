/**
 * dex_decrypt.c - protector 产物解密核心（host 可编译可测）
 */

#include "dex_decrypt.h"
#include "crypto/aes_gcm.h"
#include <string.h>

void goprotect_assemble_key(const uint8_t* const* parts, size_t parts_len,
                            uint8_t out[32]) {
    memset(out, 0, 32);
    for (size_t i = 0; i < parts_len; i++) {
        for (int j = 0; j < 32; j++) {
            out[j] ^= parts[i][j];
        }
    }
}

int goprotect_decrypt_payload(const uint8_t key[32],
                              const uint8_t nonce[12],
                              const uint8_t* ct, size_t ct_len,
                              uint8_t* out, size_t out_cap) {
    if (key == NULL || nonce == NULL || ct == NULL || out == NULL) {
        return -1;
    }
    if (ct_len < GP_GCM_TAG_LEN) {
        return -1;
    }
    if (out_cap < ct_len - GP_GCM_TAG_LEN) {
        return -1; /* 容量不足 */
    }
    return gp_aes256gcm_decrypt(key, nonce, ct, ct_len, out);
}
