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
 * 字符串解密存根：ConstObfPass 在函数入口插入调用。
 * 完整方案在 P2.2（字面量真实改写）落地后按需解密；当前保持 no-op，
 * 保证与 goprotect 处理过的 bitcode 链接不出现未解析符号。
 */
void __goprotect_decrypt_strings(void) {
    /* no-op: 字符串在 P2.2 之前不做编译期改写，无需运行时解密。 */
}
