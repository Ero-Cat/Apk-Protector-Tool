/**
 * integrity.c - 完整性校验实现（P3.1 真实化）
 *
 * 注册时对区域字节计算 FNV-1a 基线；检查时复算比对。不匹配累计失败计数
 * 并按策略响应（LOG / EXIT / ZEROIZE）。未注册 id 一次性告警放行——
 * security_hooks pass 会向所有插桩函数发检查点，未配套注册的区域按
 * "无法校验"处理而不是误报失败。
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
static volatile int g_integrity_policy = GOPROTECT_INTEGRITY_POLICY_LOG;

#define MAX_REGIONS 256
typedef struct {
    uint32_t region_id;
    const uint8_t* start;
    size_t len;
    uint32_t baseline; /* 注册时的 FNV-1a */
    int verified;
} integrity_region_t;

static integrity_region_t g_regions[MAX_REGIONS];
static int g_region_count = 0;

/* 未注册 id 的一次性告警表（防插桩检查点刷屏）。 */
#define MAX_WARNED_IDS 128
static uint32_t g_warned_ids[MAX_WARNED_IDS];
static int g_warned_count = 0;

/* FNV-1a 32 位。 */
static uint32_t gp_fnv1a(const uint8_t* data, size_t len) {
    uint32_t h = 2166136261u;
    for (size_t i = 0; i < len; i++) {
        h ^= data[i];
        h *= 16777619u;
    }
    return h;
}

void goprotect_set_integrity_policy(int policy) {
    g_integrity_policy = policy;
}

/**
 * 注册一个需要校验的内存区域：立即计算基线哈希。
 * 应在应用启动时（区域内容尚可信时）调用。
 */
void goprotect_register_region(uint32_t region_id, const void* start, size_t len) {
    if (start == NULL || len == 0 || g_region_count >= MAX_REGIONS) {
        return;
    }
    for (int i = 0; i < g_region_count; i++) {
        if (g_regions[i].region_id == region_id) {
            return; /* 重复注册以首次为准 */
        }
    }
    g_regions[g_region_count].region_id = region_id;
    g_regions[g_region_count].start = (const uint8_t*)start;
    g_regions[g_region_count].len = len;
    g_regions[g_region_count].baseline = gp_fnv1a((const uint8_t*)start, len);
    g_regions[g_region_count].verified = 0;
    g_region_count++;
}

/* 求值区域并执行策略；返回 1 表示校验通过，0 表示不匹配或未知区域。 */
static int verify_region(uint32_t region_id) {
    for (int i = 0; i < g_region_count; i++) {
        if (g_regions[i].region_id != region_id) {
            continue;
        }
        uint32_t current = gp_fnv1a(g_regions[i].start, g_regions[i].len);
        if (current == g_regions[i].baseline) {
            g_regions[i].verified = 1;
            return 1;
        }

        g_regions[i].verified = 0;
        g_integrity_failures++;
        LOGE("integrity: region %u tampered (hash mismatch)\n", region_id);

        switch (g_integrity_policy) {
        case GOPROTECT_INTEGRITY_POLICY_EXIT:
            LOGE("integrity: policy EXIT, aborting\n");
            abort();
        case GOPROTECT_INTEGRITY_POLICY_ZEROIZE: {
            /* 擦除区域内容（尽力而为；只读映射会失败，交给系统处置）。 */
            volatile uint8_t* p = (volatile uint8_t*)g_regions[i].start;
            for (size_t j = 0; j < g_regions[i].len; j++) {
                p[j] = 0;
            }
            LOGE("integrity: policy ZEROIZE applied to region %u\n", region_id);
            break;
        }
        default:
            break; /* LOG：默认路径，上面已记录 */
        }
        return 0;
    }

    /* 未注册区域：一次性告警后放行（无法校验 != 校验失败）。 */
    for (int i = 0; i < g_warned_count; i++) {
        if (g_warned_ids[i] == region_id) {
            return 0;
        }
    }
    if (g_warned_count < MAX_WARNED_IDS) {
        g_warned_ids[g_warned_count++] = region_id;
    }
    LOGW("integrity: region %u not registered, skipping\n", region_id);
    return 0;
}

/**
 * 完整性校验钩子：编译期插入到关键函数入口。
 */
void __goprotect_check_integrity(uint32_t region_id) {
    g_integrity_check_count++;
    verify_region(region_id);
}

/* 显式校验接口（测试与主动巡检用）。 */
int goprotect_verify_region(uint32_t region_id) {
    return verify_region(region_id);
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
