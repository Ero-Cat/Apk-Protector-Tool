/**
 * hooks.c - 编译期插入钩子的运行时实现
 *
 * entry_exit/const_obf 等 Pass 会对以下符号生成调用；本文件提供可链接的
 * 默认实现（保守策略：记录并继续），应用可按需替换为更强的响应。
 */

#include "goprotect.h"
#include <stdio.h>

#ifdef __ANDROID__
#include <android/log.h>
#define LOG_TAG "GoProtect"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, LOG_TAG, __VA_ARGS__)
#else
#define LOGI(...) fprintf(stdout, __VA_ARGS__)
#define LOGW(...) fprintf(stderr, __VA_ARGS__)
#endif

/* 可选的响应回调：应用注册后，__goprotect_hook 会转发事件。 */
static goprotect_hook_cb g_hook_cb = NULL;

void goprotect_set_hook_callback(goprotect_hook_cb cb) {
    g_hook_cb = cb;
}

/**
 * 入口/出口钩子：由 EntryExitPass 插入到被保护函数的入口与每个 ret 之前。
 * 默认实现做一次轻量反调试探测并转发事件。
 */
void __goprotect_hook(void) {
#ifdef GOPROTECT_RUNTIME_ENABLE_CHECKS
    __goprotect_anti_debug();
#endif
    if (g_hook_cb != NULL) {
        g_hook_cb(GOPROTECT_HOOK_EVENT_ENTER);
    }
}

/**
 * 字符串解密：ConstObfPass 在函数入口插入调用。
 *
 * Pass 侧把私有字符串全局改写为加密可写全局并导出 __gp_str_regions
 * 区域表（与本调用同 pass 产出，符号总是成对出现）；这里一次性遍历
 * 表做原位 XOR 还原。表与调用由同一 pass 生成——count 为 0 时表内是
 * 一个全零占位槽，循环自然空转。
 */
void __goprotect_decrypt_strings(void) {
    static int g_strings_decrypted = 0;
    if (g_strings_decrypted != 0) {
        return;
    }
    g_strings_decrypted = 1;

    for (int32_t i = 0; i < __gp_str_regions_count; ++i) {
        const goprotect_str_region_t* r = &__gp_str_regions[i];
        if (r->data == NULL || r->len <= 0) {
            continue;
        }
        uint8_t* bytes = (uint8_t*)(uintptr_t)r->data;
        for (int32_t j = 0; j < r->len; ++j) {
            bytes[j] ^= r->key;
        }
    }
}
