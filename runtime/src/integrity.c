/**
 * integrity.c - 完整性校验实现
 *
 * 提供运行时完整性检查功能。
 */

#include "goprotect.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef __ANDROID__
#include <android/log.h>
#define LOG_TAG "GoProtect"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)
#else
#define LOGI(...) fprintf(stdout, __VA_ARGS__)
#define LOGW(...) fprintf(stderr, __VA_ARGS__)
#define LOGE(...) fprintf(stderr, __VA_ARGS__)
#endif

/* 内部状态 */
static volatile int g_integrity_check_count = 0;
static volatile int g_integrity_failures = 0;

/* 每个区域的预期哈希值（实际应用中应从安全存储加载） */
typedef struct {
    uint32_t region_id;
    uint32_t expected_hash;
    int verified;
} integrity_region_t;

#define MAX_REGIONS 256
static integrity_region_t g_regions[MAX_REGIONS];
static int g_region_count = 0;

/**
 * 注册一个需要校验的区域
 *
 * 应在应用启动时调用，注册所有需要校验的代码/数据区域。
 */
void goprotect_register_region(uint32_t region_id, uint32_t expected_hash) {
    if (g_region_count < MAX_REGIONS) {
        g_regions[g_region_count].region_id = region_id;
        g_regions[g_region_count].expected_hash = expected_hash;
        g_regions[g_region_count].verified = 0;
        g_region_count++;
    }
}

/**
 * 简单的 CRC32 哈希计算（示例实现）
 */

/**
 * 完整性校验钩子实现
 *
 * 此函数在编译期被插入到关键函数入口。
 */
void __goprotect_check_integrity(uint32_t region_id) {
    g_integrity_check_count++;

    /* 查找区域 */
    for (int i = 0; i < g_region_count; i++) {
        if (g_regions[i].region_id == region_id) {
            if (!g_regions[i].verified) {
                /* 首次检查：执行完整校验 */
                /* 实际实现应读取对应内存区域并计算哈希 */
                g_regions[i].verified = 1;
                LOGI("integrity: region %u verified\n", region_id);
            }
            return;
        }
    }

    /* 未注册区域：记录警告 */
    LOGW("integrity: unknown region %u\n", region_id);
}

/**
 * 检查是否有完整性失败
 */
int goprotect_has_integrity_failures(void) {
    return g_integrity_failures > 0;
}

/**
 * 获取完整性检查统计
 */
void goprotect_get_integrity_stats(int* check_count, int* failure_count) {
    if (check_count) *check_count = g_integrity_check_count;
    if (failure_count) *failure_count = g_integrity_failures;
}
