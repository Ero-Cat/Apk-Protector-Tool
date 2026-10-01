/**
 * vm_entry.c - VM 入口点实现
 *
 * 提供虚拟化函数的运行时解释执行框架。
 *
 * 字节码经编译期随机化（opcode 重映射）并 XOR 加密后嵌入模块，入口从
 * 元数据 JSON 中读取 bytecode_len / key_pad / opcodes 解码表，解密后按
 * 标准 opcode 解释执行。
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

/* 标准 opcode（与 passes/vmp/opcodes.go 的 BaseOpcodes 保持同步，
 * 一致性由 passes/vmp 的构建期测试校验）。 */
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
#define OP_CMP_ULT 0x46
#define OP_CMP_UGT 0x47
#define OP_CMP_ULE 0x48
#define OP_CMP_UGE 0x49

#define OP_JMP 0x50
#define OP_JZ  0x51
#define OP_JNZ 0x52

#define OP_CALL_EXT 0x60

#define OP_NOP 0xFE
#define OP_RET 0xFF

/* 标准助记符表：元数据 opcodes 映射（助记符 -> 随机化字节）据此反转。 */
typedef struct {
    const char* name;
    uint8_t op;
} opcode_entry_t;

static const opcode_entry_t kOpcodeTable[] = {
    {"PUSH_CONST", OP_PUSH_CONST},
    {"PUSH_ARG",   OP_PUSH_ARG},
    {"PUSH_LOCAL", OP_PUSH_LOCAL},
    {"POP",        OP_POP},
    {"DUP",        OP_DUP},
    {"ADD",        OP_ADD},
    {"SUB",        OP_SUB},
    {"MUL",        OP_MUL},
    {"DIV",        OP_DIV},
    {"MOD",        OP_MOD},
    {"NEG",        OP_NEG},
    {"AND",        OP_AND},
    {"OR",         OP_OR},
    {"XOR",        OP_XOR},
    {"NOT",        OP_NOT},
    {"SHL",        OP_SHL},
    {"SHR",        OP_SHR},
    {"LOAD",       OP_LOAD},
    {"STORE",      OP_STORE},
    {"CMP_EQ",     OP_CMP_EQ},
    {"CMP_NE",     OP_CMP_NE},
    {"CMP_LT",     OP_CMP_LT},
    {"CMP_LE",     OP_CMP_LE},
    {"CMP_GT",     OP_CMP_GT},
    {"CMP_GE",     OP_CMP_GE},
    {"CMP_ULT",    OP_CMP_ULT},
    {"CMP_UGT",    OP_CMP_UGT},
    {"CMP_ULE",    OP_CMP_ULE},
    {"CMP_UGE",    OP_CMP_UGE},
    {"JMP",        OP_JMP},
    {"JZ",         OP_JZ},
    {"JNZ",        OP_JNZ},
    {"CALL_EXT",   OP_CALL_EXT},
    {"NOP",        OP_NOP},
    {"RET",        OP_RET},
};
#define kOpcodeCount (sizeof(kOpcodeTable) / sizeof(kOpcodeTable[0]))

/* VM 上下文 */
typedef struct {
    int32_t stack[VM_STACK_SIZE];
    int sp;                         /* 栈指针 */
    int32_t locals[VM_LOCAL_SIZE];  /* 局部变量 */
    const uint8_t* bytecode;        /* 解码并解密后的字节码 */
    size_t bytecode_len;            /* 字节码长度（来自元数据） */
    size_t ip;                      /* 指令指针 */
    int halted;                     /* 停止标志 */
    uint8_t decode[256];            /* 随机化字节 -> 标准 opcode */
} vm_context_t;

/* 外部函数表项 */
typedef void (*ext_func_t)(vm_context_t* ctx, int argc);

static ext_func_t g_ext_funcs[VM_MAX_EXT_FUNCS];
static int g_ext_func_count = 0;

/* 静态密钥片段：与应用共享的构建期常量，经 goprotect_set_static_key 注册。
 * 默认值与编译期 VirtualizePass.staticKeyByte 的缺省一致（0x5A）。 */
static uint8_t g_static_key = 0x5A;

void goprotect_set_static_key(uint8_t key) {
    g_static_key = key;
}

/* ==========================================================================
 * 元数据 JSON 最小解析（元数据形状由编译期 metadataJSON 生成，结构固定）
 * ========================================================================== */

/* 定位 "key": 之后的值起始位置；找不到返回 NULL。 */
static const char* json_value_of(const char* json, const char* key) {
    size_t klen = strlen(key);
    const char* p = json;
    while ((p = strstr(p, key)) != NULL) {
        /* 确认是独立的键名：前引号 + 后引号 */
        if (p > json && p[-1] == '"' && p[klen] == '"') {
            const char* v = p + klen + 1;
            while (*v == ' ' || *v == '\t') v++;
            if (*v == ':') {
                v++;
                while (*v == ' ' || *v == '\t') v++;
                return v;
            }
        }
        p += klen;
    }
    return NULL;
}

static int json_int_of(const char* json, const char* key, int def) {
    const char* v = json_value_of(json, key);
    if (v == NULL) return def;
    return (int)strtol(v, NULL, 10);
}

static int json_bool_of(const char* json, const char* key, int def) {
    const char* v = json_value_of(json, key);
    if (v == NULL) return def;
    return *v == 't';
}

/* 从 "opcodes":{...} 对象构建 decode 表（随机化字节 -> 标准 opcode）。 */
static void json_build_decode(const char* json, uint8_t* decode) {
    size_t i;
    /* 缺省恒等映射：未随机化的字节按原值解释。 */
    for (i = 0; i < 256; i++) decode[i] = (uint8_t)i;

    const char* v = json_value_of(json, "\"opcodes\"");
    if (v == NULL || *v != '{') return;

    const char* p = v + 1;
    while (*p && *p != '}') {
        char name[32];
        size_t n = 0;
        /* 跳过空白与逗号 */
        while (*p == ' ' || *p == ',' || *p == '\n' || *p == '\t') p++;
        if (*p != '"' || *p == '}') break;
        p++;
        while (*p && *p != '"' && n + 1 < sizeof(name)) name[n++] = *p++;
        name[n] = '\0';
        if (*p != '"') break;
        p++;
        while (*p == ' ' || *p == '\t') p++;
        if (*p != ':') break;
        p++;
        while (*p == ' ' || *p == '\t') p++;
        long randomized = strtol(p, (char**)&p, 10);
        for (i = 0; i < kOpcodeCount; i++) {
            if (strcmp(kOpcodeTable[i].name, name) == 0) {
                decode[(uint8_t)randomized] = kOpcodeTable[i].op;
                break;
            }
        }
    }
}

/* ==========================================================================
 * 解释器
 * ========================================================================== */

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
 * VM 解释器主循环：先经 decode 表把随机化字节还原为标准 opcode。
 */
static void vm_execute(vm_context_t* ctx) {
    while (!ctx->halted && ctx->ip < ctx->bytecode_len) {
        uint8_t raw = read_u8(ctx);
        uint8_t op = ctx->decode[raw];
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

        case OP_LOAD: {
            /* 地址在栈顶：按 4 字节对齐读取 */
            b = VM_POP(ctx);
            VM_PUSH(ctx, *(int32_t*)(intptr_t)b);
            break;
        }

        case OP_STORE: {
            int32_t addr = VM_POP(ctx);
            int32_t val = VM_POP(ctx);
            *(int32_t*)(intptr_t)addr = val;
            break;
        }

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

        case OP_CMP_ULT:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, (uint32_t)a < (uint32_t)b ? 1 : 0);
            break;

        case OP_CMP_UGT:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, (uint32_t)a > (uint32_t)b ? 1 : 0);
            break;

        case OP_CMP_ULE:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, (uint32_t)a <= (uint32_t)b ? 1 : 0);
            break;

        case OP_CMP_UGE:
            b = VM_POP(ctx);
            a = VM_POP(ctx);
            VM_PUSH(ctx, (uint32_t)a >= (uint32_t)b ? 1 : 0);
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
            } else {
                LOGW("vm: ext func %u not registered\n", (unsigned)fn_id);
            }
            break;
        }

        case OP_NOP:
            break;

        case OP_RET:
            ctx->halted = 1;
            break;

        default:
            LOGW("vm: unknown opcode 0x%02x (raw 0x%02x) at ip=%zu\n", op, raw, ctx->ip - 1);
            ctx->halted = 1;
            break;
        }
    }
}

/**
 * 共享 VM 入口：解析元数据 -> 构建解码表 -> 解密 -> 解释执行。
 */
static void vm_entry_run(const uint8_t* bytecode, const char* meta) {
    vm_context_t ctx;
    memset(&ctx, 0, sizeof(ctx));

    size_t len = (size_t)json_int_of(meta, "\"bytecode_len\"", 0);
    int encrypted = json_bool_of(meta, "\"encrypted\"", 0);
    uint8_t key_pad = (uint8_t)json_int_of(meta, "\"key_pad\"", 0);
    json_build_decode(meta, ctx.decode);

    if (len == 0) {
        LOGE("vm: metadata missing bytecode_len, refusing to run\n");
        return;
    }

    /* 解密到独立缓冲区：模块内的全局字节码是只读的。 */
    uint8_t* code = (uint8_t*)malloc(len);
    if (code == NULL) {
        LOGE("vm: out of memory for %zu bytes of bytecode\n", len);
        return;
    }
    memcpy(code, bytecode, len);
    if (encrypted) {
        uint8_t key = g_static_key ^ key_pad;
        for (size_t i = 0; i < len; i++) {
            code[i] ^= key;
        }
    }

    ctx.bytecode = code;
    ctx.bytecode_len = len;
    ctx.ip = 0;
    ctx.sp = 0;
    ctx.halted = 0;

    vm_execute(&ctx);

    free(code);
}

/**
 * VM 入口点 - VM A（加密版本）
 */
void __goprotect_vm_entry_encrypted_vm_a(const uint8_t* bytecode, const char* meta) {
    vm_entry_run(bytecode, meta);
}

/**
 * VM 入口点 - VM B（加密版本；opcode 映射按各自元数据独立解码）
 */
void __goprotect_vm_entry_encrypted_vm_b(const uint8_t* bytecode, const char* meta) {
    vm_entry_run(bytecode, meta);
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
