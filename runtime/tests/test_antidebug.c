/**
 * test_antidebug.c - 端口探测测试（P3.2）
 *
 * 双向验证 goprotect_probe_port：临时监听端口 -> 阳性；关闭后 -> 阴性。
 *
 * 编译运行：
 *   clang -std=c11 -Wall -Wextra -Werror -I runtime/include \
 *     runtime/tests/test_antidebug.c -o /tmp/test_antidebug && /tmp/test_antidebug
 */
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <unistd.h>

#include "../src/antidebug.c"

static int g_failures = 0;

#define CHECK(cond)                                                     \
    do {                                                                \
        if (!(cond)) {                                                  \
            fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, #cond); \
            g_failures++;                                               \
        }                                                               \
    } while (0)

static int g_test_listen_fd = -1;

/* 在 127.0.0.1 上绑定一个临时监听 socket，返回端口；失败返回 0。 */
static uint16_t open_listener(void) {
    int fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd < 0) return 0;
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof addr);
    addr.sin_family = AF_INET;
    addr.sin_port = 0; /* 内核分配临时端口 */
    addr.sin_addr.s_addr = htonl(0x7F000001u);
    if (bind(fd, (struct sockaddr*)&addr, sizeof addr) != 0 || listen(fd, 1) != 0) {
        close(fd);
        return 0;
    }
    socklen_t slen = sizeof addr;
    if (getsockname(fd, (struct sockaddr*)&addr, &slen) != 0) {
        close(fd);
        return 0;
    }
    g_test_listen_fd = fd;
    return ntohs(addr.sin_port);
}

int main(void) {
    /* 1. 打开的监听端口必须探测为阳性。 */
    uint16_t port = open_listener();
    CHECK(port != 0);
    if (port != 0) {
        CHECK(goprotect_probe_port(port) == 1);

        /* 2. 关闭后同一端口应转为阴性（监听 socket 无 TIME_WAIT 残留）。
         * 极小概率被其他进程抢占——重试几次容忍调度抖动。 */
        close(g_test_listen_fd);
        int saw_closed = 0;
        for (int i = 0; i < 3 && !saw_closed; i++) {
            if (goprotect_probe_port(port) == 0) saw_closed = 1;
        }
        CHECK(saw_closed);
    }

    if (g_failures == 0) {
        printf("test_antidebug: all checks passed\n");
        return 0;
    }
    printf("test_antidebug: %d check(s) failed\n", g_failures);
    return 1;
}
