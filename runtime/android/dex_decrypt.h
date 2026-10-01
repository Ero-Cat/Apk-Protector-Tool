/**
 * dex_decrypt.h - protector 产物解密核心（host 可编译）
 *
 * 输入为 protector 的产物形状：assets/protector/dex/<name>.enc（AES-256-GCM，
 * 密文后追加 16 字节 tag）+ metadata.json 中的 nonce（base64）。密钥按
 * ADR-0001 由调用方注入——demo 采用"拆分常量异或拼装"，生产可换成服务端
 * 下发 / 签名派生 / 硬件密钥。
 */

#ifndef GOPROTECT_DEX_DECRYPT_H
#define GOPROTECT_DEX_DECRYPT_H

#include <stddef.h>
#include <stdint.h>

/* 从拆分密钥分片异或拼装出 32 字节 AES 密钥。
 * 任一分片不少于 32 字节时行为未定义由调用方保证（demo 分片定长 32）。 */
void goprotect_assemble_key(const uint8_t* const* parts, size_t parts_len,
                            uint8_t out[32]);

/* 解密 payload：ct 为密文+tag；nonce 12 字节（来自 metadata 的 base64 解码）。
 * 成功返回明文长度，失败（验签不过/参数非法）返回 -1。
 * 注意：compress_before_encrypt=true 的产物是"先 Deflate 后加密"，本核心
 * 只做解密——demo 约定关闭该选项，或由调用方接 zlib inflate。 */
int goprotect_decrypt_payload(const uint8_t key[32],
                              const uint8_t nonce[12],
                              const uint8_t* ct, size_t ct_len,
                              uint8_t* out, size_t out_cap);

#endif /* GOPROTECT_DEX_DECRYPT_H */
