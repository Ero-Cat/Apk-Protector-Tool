/**
 * antidebug.c - 反调试实现
 *
 * 提供运行时反调试检测功能。
 */

#include "goprotect.h"
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <stdint.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <sys/select.h>
#include <netinet/in.h>
#include <fcntl.h>
#include <unistd.h>

#ifdef __ANDROID__
#include <android/log.h>
#include <unistd.h>
#include <sys/ptrace.h>
#include <signal.h>
#include <pthread.h>

#define LOG_TAG "GoProtect"
#define LOGI(...) __android_log_print(ANDROID_LOG_INFO, LOG_TAG, __VA_ARGS__)
#define LOGW(...) __android_log_print(ANDROID_LOG_WARN, LOG_TAG, __VA_ARGS__)
#define LOGE(...) __android_log_print(ANDROID_LOG_ERROR, LOG_TAG, __VA_ARGS__)
#else
#define LOGI(...) fprintf(stdout, __VA_ARGS__)
#define LOGW(...) fprintf(stderr, __VA_ARGS__)
#define LOGE(...) fprintf(stderr, __VA_ARGS__)
#endif

/* 检测状态 */
static volatile int g_debug_check_count = 0;
static volatile int g_debugger_detected = 0;
static volatile int g_frida_detected = 0;
static volatile int g_emulator_detected = 0;

/**
 * 检测 TracerPid（调试器附加检测）
 */
static int check_tracer_pid(void) {
#ifdef __ANDROID__
    char path[64];
    char line[256];
    int tracer_pid = 0;

    snprintf(path, sizeof(path), "/proc/%d/status", getpid());
    FILE* f = fopen(path, "r");
    if (f == NULL) return 0;

    while (fgets(line, sizeof(line), f)) {
        if (strncmp(line, "TracerPid:", 10) == 0) {
            tracer_pid = atoi(line + 10);
            break;
        }
    }
    fclose(f);

    return tracer_pid != 0;
#else
    return 0;
#endif
}

/**
 * 探测本机 TCP 端口是否有监听（P3.2，跨平台）。
 *
 * 非阻塞 connect + 150ms select：端口关闭时本地回环立即 ECONNREFUSED，
 * 不产生等待开销；连接成功即视为监听存在。
 */
int goprotect_probe_port(uint16_t port) {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) {
        return 0;
    }

    int flags = fcntl(fd, F_GETFL, 0);
    if (flags >= 0) {
        fcntl(fd, F_SETFL, flags | O_NONBLOCK);
    }

    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = htonl(0x7F000001u); /* 127.0.0.1 */

    int open = 0;
    int rc = connect(fd, (struct sockaddr*)&addr, sizeof(addr));
    if (rc == 0) {
        open = 1;
    } else if (rc < 0 && (errno == EINPROGRESS || errno == EWOULDBLOCK)) {
        struct timeval tv;
        tv.tv_sec = 0;
        tv.tv_usec = 150000; /* 150 ms */
        fd_set wset;
        FD_ZERO(&wset);
        FD_SET(fd, &wset);
        if (select(fd + 1, NULL, &wset, NULL, &tv) > 0) {
            int soerr = 0;
            socklen_t slen = sizeof(soerr);
            if (getsockopt(fd, SOL_SOCKET, SO_ERROR, &soerr, &slen) == 0 && soerr == 0) {
                open = 1;
            }
        }
    }

    close(fd);
    return open;
}

/**
 * 检测 Frida 注入
 *
 * 通过检查常见的 Frida 特征：
 * - frida-server / frida-gadget 默认端口 (27042 / 27043)
 * - /proc/self/maps 中的 frida / gadget 字符串（Android）
 */
static const uint16_t kFridaPorts[] = {27042, 27043};

static int check_frida(void) {
#ifdef __ANDROID__
    char line[512];
    FILE* f = fopen("/proc/self/maps", "r");
    if (f != NULL) {
        while (fgets(line, sizeof(line), f)) {
            if (strstr(line, "frida") || strstr(line, "gadget")) {
                fclose(f);
                return 1;
            }
        }
        fclose(f);
    }
#endif

    /* 端口探测在 host 与 Android 上都可用。 */
    for (size_t i = 0; i < sizeof(kFridaPorts) / sizeof(kFridaPorts[0]); i++) {
        if (goprotect_probe_port(kFridaPorts[i])) {
            return 1;
        }
    }

    return 0;
}

/**
 * 检测模拟器环境
 */
static int check_emulator(void) {
#ifdef __ANDROID__
    /* 检查常见模拟器特征文件 */
    const char* emulator_files[] = {
        "/dev/socket/qemud",
        "/dev/qemu_pipe",
        "/system/lib/libc_malloc_debug_qemu.so",
        "/sys/qemu_trace",
        "/system/bin/qemu-props",
        NULL
    };

    for (int i = 0; emulator_files[i]; i++) {
        if (access(emulator_files[i], F_OK) == 0) {
            return 1;
        }
    }

    /* 检查 CPU 信息 */
    FILE* f = fopen("/proc/cpuinfo", "r");
    if (f) {
        char line[256];
        while (fgets(line, sizeof(line), f)) {
            if (strstr(line, "goldfish") || strstr(line, "ranchu")) {
                fclose(f);
                return 1;
            }
        }
        fclose(f);
    }

    return 0;
#else
    return 0;
#endif
}

/**
 * 反调试钩子实现
 */
void __goprotect_anti_debug(void) {
    g_debug_check_count++;

    /* 仅周期性检测，避免性能影响 */
    if (g_debug_check_count % 10 != 1) {
        return;
    }

    /* TracerPid 检测 */
    if (check_tracer_pid()) {
        if (!g_debugger_detected) {
            g_debugger_detected = 1;
            LOGW("antidebug: debugger detected\n");
        }
    }

    /* Frida 检测 */
    if (check_frida()) {
        if (!g_frida_detected) {
            g_frida_detected = 1;
            LOGW("antidebug: frida detected\n");
        }
    }

    /* 模拟器检测（首次检查） */
    if (g_debug_check_count == 1) {
        if (check_emulator()) {
            g_emulator_detected = 1;
            LOGI("antidebug: emulator environment\n");
        }
    }
}

/**
 * 获取检测状态
 */
int goprotect_is_debugger_detected(void) {
    return g_debugger_detected;
}

int goprotect_is_frida_detected(void) {
    return g_frida_detected;
}

int goprotect_is_emulator(void) {
    return g_emulator_detected;
}
