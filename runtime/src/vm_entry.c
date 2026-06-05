/**
 * vm_entry.c - VM 入口点实现
 *
 * 提供虚拟化函数的运行时解释执行框架。
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

/* VM 配置 */
#define VM_STACK_SIZE 256
#define VM_LOCAL_SIZE 64
#define VM_MAX_EXT_FUNCS 128

/* 操作码定义（与 passes/vmp/opcodes.go 保持同步） */
#define OP_PUSH_CONST 0x01
#define OP_PUSH_ARG   0x02
#define OP_PUSH_LOCAL 0x03
#define OP_POP        0x04
#define OP_DUP        0x05

#define OP_ADD 0x10
#define OP_SUB 0x11
#define OP_MUL 0x12
#define OP_DIV 0x13
#define OP_MOD 0x14
#define OP_NEG 0x15

#define OP_AND 0x20
#define OP_OR  0x21
#define OP_XOR 0x22
#define OP_NOT 0x23
#define OP_SHL 0x24
#define OP_SHR 0x25

#define OP_LOAD  0x30
#define OP_STORE 0x31

#define OP_CMP_EQ  0x40
#define OP_CMP_NE  0x41
#define OP_CMP_LT  0x42
#define OP_CMP_LE  0x43
#define OP_CMP_GT  0x44
#define OP_CMP_GE  0x45

#define OP_JMP 0x50
#define OP_JZ  0x51
#define OP_JNZ 0x52

#define OP_CALL_EXT 0x60

#define OP_NOP 0xFE
#define OP_RET 0xFF

/* VM 上下文 */
typedef struct {
    int32_t stack[VM_STACK_SIZE];
    int sp;                         /* 栈指针 */
    int32_t locals[VM_LOCAL_SIZE];  /* 局部变量 */
    const uint8_t* bytecode;        /* 字节码指针 */
    size_t bytecode_len;            /* 字节码长度 */
    size_t ip;                      /* 指令指针 */
    int halted;                     /* 停止标志 */
} vm_context_t;

/* 外部函数表项 */
typedef void (*ext_func_t)(vm_context_t* ctx, int argc);

static ext_func_t g_ext_funcs[VM_MAX_EXT_FUNCS];
static int g_ext_func_count = 0;

/* 栈操作宏 */
#define VM_PUSH(ctx, val) do { \
    if ((ctx)->sp < VM_STACK_SIZE) { \
        (ctx)->stack[(ctx)->sp++] = (val); \
    } \
} while(0)

#define VM_POP(ctx) (((ctx)->sp > 0) ? (ctx)->stack[--(ctx)->sp] : 0)

#define VM_PEEK(ctx) (((ctx)->sp > 0) ? (ctx)->stack[(ctx)->sp - 1] : 0)

/* 读取字节码辅助函数 */
static inline uint8_t read_u8(vm_context_t* ctx) {
    if (ctx->ip < ctx->bytecode_len) {
        return ctx->bytecode[ctx->ip++];
    }
    return 0;
}

static inline int16_t read_i16(vm_context_t* ctx) {
    uint8_t lo = read_u8(ctx);
    uint8_t hi = read_u8(ctx);
    return (int16_t)((hi << 8) | lo);
}

static inline int32_t read_i32(vm_context_t* ctx) {
    uint8_t b0 = read_u8(ctx);
    uint8_t b1 = read_u8(ctx);
    uint8_t b2 = read_u8(ctx);
    uint8_t b3 = read_u8(ctx);
    return (int32_t)(b0 | (b1 << 8) | (b2 << 16) | (b3 << 24));
}

/**
 * VM 解释器主循环
 */
static void vm_execute(vm_context_t* ctx) {
    while (!ctx->halted && ctx->ip < ctx->bytecode_len) {
        uint8_t op = read_u8(ctx);
        int32_t a, b, result;

        switch (op) {
        case OP_PUSH_CONST:
            VM_PUSH(ctx, read_i32(ctx));
            break;

        case OP_PUSH_ARG: {
            uint8_t idx = read_u8(ctx);
            /* 参数从 locals 开始存储 */
            VM_PUSH(ctx, ctx->locals[idx]);
            break;
        }

        case OP_PUSH_LOCAL: {
            uint8_t idx = read_u8(ctx);
            VM_PUSH(ctx, ctx->locals[idx]);
            break;
        }

        case OP_POP:
            VM_POP(ctx);
            break;

        case OP_DUP:
            VM_PUSH(ctx, VM_PEEK(ctx));
            break;

        case OP_ADD:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a + b);
            break;

        case OP_SUB:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a - b);
            break;

        case OP_MUL:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a * b);
            break;

        case OP_DIV:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, b != 0 ? a / b : 0);
            break;

        case OP_MOD:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, b != 0 ? a % b : 0);
            break;

        case OP_NEG:
            a = VM_POP(ctx);
            VM_PUSH(ctx, -a);
            break;

        case OP_AND:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a & b);
            break;

        case OP_OR:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a | b);
            break;

        case OP_XOR:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a ^ b);
            break;

        case OP_NOT:
            a = VM_POP(ctx);
            VM_PUSH(ctx, ~a);
            break;

        case OP_SHL:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a << b);
            break;

        case OP_SHR:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, (uint32_t)a >> b);
            break;

        case OP_CMP_EQ:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a == b ? 1 : 0);
            break;

        case OP_CMP_NE:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a != b ? 1 : 0);
            break;

        case OP_CMP_LT:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a < b ? 1 : 0);
            break;

        case OP_CMP_LE:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a <= b ? 1 : 0);
            break;

        case OP_CMP_GT:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a > b ? 1 : 0);
            break;

        case OP_CMP_GE:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, a >= b ? 1 : 0);
            break;

        case OP_JMP: {
            int16_t offset = read_i16(ctx);
            ctx->ip += offset;
            break;
        }

        case OP_JZ: {
            int16_t offset = read_i16(ctx);
            a = VM_POP(ctx);
            if (a == 0) {
                ctx->ip += offset;
            }
            break;
        }

        case OP_JNZ: {
            int16_t offset = read_i16(ctx);
            a = VM_POP(ctx);
            if (a != 0) {
                ctx->ip += offset;
            }
            break;
        }

        case OP_CALL_EXT: {
            uint16_t fn_id = read_u8(ctx) | (read_u8(ctx) << 8);
            uint8_t argc = read_u8(ctx);
            if (fn_id < g_ext_func_count && g_ext_funcs[fn_id]) {
                g_ext_funcs[fn_id](ctx, argc);
            }
            break;
        }

        case OP_NOP:
            break;

        case OP_RET:
            ctx->halted = 1;
            break;

        default:
            LOGW("vm: unknown opcode 0x%02x at ip=%zu\n", op, ctx->ip - 1);
            ctx->halted = 1;
            break;
        }
    }
}

/**
 * VM 入口点 - VM A（加密版本）
 */
void __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode) {
    /* 此处应解密字节码，密钥从元数据获取 */
    /* 简化实现：直接执行 */
    vm_context_t ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.bytecode = bytecode;
    ctx.bytecode_len = 1024; /* 实际应从元数据读取 */
    ctx.ip = 0;
    ctx.sp = 0;
    ctx.halted = 0;

    vm_execute(&ctx);
}

/**
 * VM 入口点 - VM B（加密版本）
 */
void __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode) {
    /* 与 VM A 类似，可有不同的 opcode 映射 */
    vm_context_t ctx;
    memset(&ctx, 0, sizeof(ctx));
    ctx.bytecode = bytecode;
    ctx.bytecode_len = 1024;
    ctx.ip = 0;
    ctx.sp = 0;
    ctx.halted = 0;

    vm_execute(&ctx);
}

/**
 * 注册外部函数
 */
int goprotect_register_ext_func(int id, ext_func_t func) {
    if (id >= 0 && id < VM_MAX_EXT_FUNCS) {
        g_ext_funcs[id] = func;
        if (id >= g_ext_func_count) {
            g_ext_func_count = id + 1;
        }
        return 0;
    }
    return -1;
}
