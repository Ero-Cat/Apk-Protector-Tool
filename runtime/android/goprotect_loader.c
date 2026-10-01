/**
 * goprotect_loader.c - NDK demo 加载器（Android only）
 *
 * 组合方式（ADR-0001）：
 *   1. 从 APK assets 读取 assets/protector/metadata.json 与
 *      assets/protector/dex/classes.dex.enc；
 *   2. 密钥由拆分常量异或拼装（编译期注入 kKeyParts，发布流程负责生成）；
 *   3. dex_decrypt 核心 AES-256-GCM 解密；
 *   4. 明文 dex 交回 Java 侧经 InMemoryDexClassLoader 装载。
 *
 * 构建：NDK + CMakeLists.txt（见同目录）。host 上仅编译 dex_decrypt.c 核心
 * （tests/ 覆盖），本文件整体位于 __ANDROID__ 门后。
 *
 * 约定：protector 侧 compress_before_encrypt=false（Deflate 需接 zlib）。
 */

#ifdef __ANDROID__

#include <jni.h>
#include <string.h>
#include <stdlib.h>
#include <android/asset_manager.h>
#include <android/asset_manager_jni.h>

#include "dex_decrypt.h"
#include "crypto/aes_gcm.h"

/* 拆分密钥分片（示例占位值，发布流程生成后替换）。
 * kKeyParts[0] ^ kKeyParts[1] == 外置 .key 文件中的 aes_key。 */
static const uint8_t kKeyPart0[32] = {
    0x11,0x22,0x33,0x44,0x55,0x66,0x77,0x88,0x99,0xaa,0xbb,0xcc,0xdd,0xee,0xff,0x00,
    0x01,0x02,0x03,0x04,0x05,0x06,0x07,0x08,0x09,0x0a,0x0b,0x0c,0x0d,0x0e,0x0f,0x10
};
static const uint8_t kKeyPart1[32] = {
    0xf0,0xe0,0xd0,0xc0,0xb0,0xa0,0x90,0x80,0x70,0x60,0x50,0x40,0x30,0x20,0x10,0x00,
    0x1f,0x2e,0x3d,0x4c,0x5b,0x6a,0x79,0x88,0x97,0xa6,0xb5,0xc4,0xd3,0xe2,0xf1,0x00
};

static uint8_t* read_asset(JNIEnv* env, jobject assetManager,
                           const char* path, size_t* out_len) {
    AAssetManager* mgr = AAssetManager_fromJava(env, assetManager);
    if (mgr == NULL) return NULL;
    AAsset* asset = AAssetManager_open(mgr, path, AASSET_MODE_BUFFER);
    if (asset == NULL) return NULL;

    off_t len = AAsset_getLength(asset);
    uint8_t* buf = (uint8_t*)malloc((size_t)len);
    if (buf == NULL) {
        AAsset_close(asset);
        return NULL;
    }
    int read = AAsset_read(asset, buf, (size_t)len);
    AAsset_close(asset);
    if (read != len) {
        free(buf);
        return NULL;
    }
    *out_len = (size_t)len;
    return buf;
}

/* 定位 metadata.json 中第一个 "nonce": "..." 的 base64 值并解码。 */
static int find_nonce(const uint8_t* meta, size_t meta_len,
                      uint8_t nonce[GP_GCM_NONCE_LEN]) {
    (void)meta_len;
    const char* key = strstr((const char*)meta, "\"nonce\"");
    if (key == NULL) return -1;
    const char* colon = strchr(key + 7, ':');
    if (colon == NULL) return -1;
    const char* v = strchr(colon, '"');
    if (v == NULL) return -1;
    v++;
    const char* end = strchr(v, '"');
    if (end == NULL || (size_t)(end - v) != 16) return -1; /* 12B -> 16 chars */

    int8_t dec[256];
    memset(dec, -1, sizeof dec);
    const char* alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    for (int i = 0; i < 64; i++) dec[(uint8_t)alphabet[i]] = (int8_t)i;

    uint8_t acc = 0, nbits = 0, out_idx = 0;
    for (const char* c = v; c < end; c++) {
        int8_t d = dec[(uint8_t)*c];
        if (d < 0) return -1;
        acc = (uint8_t)((acc << 6) | (uint8_t)d);
        nbits += 6;
        if (nbits >= 8) {
            nbits -= 8;
            if (out_idx >= GP_GCM_NONCE_LEN) return -1;
            nonce[out_idx++] = (uint8_t)(acc >> nbits);
        }
    }
    return (out_idx == GP_GCM_NONCE_LEN) ? 0 : -1;
}

JNIEXPORT jbyteArray JNICALL
Java_com_goprotect_demo_GoprotectLoader_nativeLoadDex(JNIEnv* env, jobject thiz,
                                                      jobject assetManager) {
    (void)thiz;

    size_t ct_len = 0, meta_len = 0;
    uint8_t* ct = read_asset(env, assetManager, "protector/dex/classes.dex.enc", &ct_len);
    uint8_t* meta = read_asset(env, assetManager, "protector/metadata.json", &meta_len);
    if (ct == NULL || meta == NULL) {
        free(ct);
        free(meta);
        return NULL; /* assets 路径不同时按需调整 */
    }

    uint8_t nonce[GP_GCM_NONCE_LEN];
    if (find_nonce(meta, meta_len, nonce) != 0) {
        free(ct);
        free(meta);
        return NULL;
    }

    uint8_t key[32];
    const uint8_t* parts[2] = { kKeyPart0, kKeyPart1 };
    goprotect_assemble_key(parts, 2, key);

    if (ct_len <= GP_GCM_TAG_LEN) {
        free(ct);
        free(meta);
        return NULL;
    }
    size_t plain_len = ct_len - GP_GCM_TAG_LEN;
    uint8_t* plain = (uint8_t*)malloc(plain_len);
    if (plain == NULL ||
        goprotect_decrypt_payload(key, nonce, ct, ct_len, plain, plain_len) < 0) {
        free(plain);
        free(ct);
        free(meta);
        return NULL; /* 验签失败：密钥不匹配或数据被篡改 */
    }

    jbyteArray result = (*env)->NewByteArray(env, (jsize)plain_len);
    if (result != NULL) {
        (*env)->SetByteArrayRegion(env, result, 0, (jsize)plain_len, (const jbyte*)plain);
    }
    free(plain);
    free(ct);
    free(meta);
    return result;
}

#endif /* __ANDROID__ */
