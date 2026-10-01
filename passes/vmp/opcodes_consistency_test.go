package vmp

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// runtimeSourcePath 指向 C 运行时源码；测试据此校验两侧 opcode 定义一致。
func runtimeSourcePath(t *testing.T) string {
	t.Helper()
	// passes/vmp -> runtime/src
	path := filepath.Join("..", "..", "..", "runtime", "src", "vm_entry.c")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("runtime source not found: %v", err)
	}
	return path
}

// TestOpcodeTableMatchesC 验证 Go BaseOpcodes 与 C kOpcodeTable/#define 完全一致。
func TestOpcodeTableMatchesC(t *testing.T) {
	source, err := os.ReadFile(runtimeSourcePath(t))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	// C 侧助记符表条目：{"CMP_EQ",     OP_CMP_EQ},
	entryRe := regexp.MustCompile(`\{"([A-Z_]+)",\s*OP_[A-Z_]+\}`)
	cNames := map[string]bool{}
	for _, m := range entryRe.FindAllStringSubmatch(text, -1) {
		cNames[m[1]] = true
	}

	// C 侧 #define：OP_CMP_EQ 0x40
	defineRe := regexp.MustCompile(`#define\s+(OP_[A-Z_]+)\s+(0x[0-9A-Fa-f]+)`)
	cDefines := map[string]byte{}
	for _, m := range defineRe.FindAllStringSubmatch(text, -1) {
		name := strings.TrimPrefix(m[1], "OP_")
		value := parseHex(t, m[2])
		cDefines[name] = value
	}

	if len(cNames) != len(BaseOpcodes) {
		t.Fatalf("C opcode table has %d entries, Go has %d", len(cNames), len(BaseOpcodes))
	}
	for op, info := range BaseOpcodes {
		if !cNames[info.Name] {
			t.Errorf("opcode %s (0x%02x) missing from C kOpcodeTable", info.Name, op)
			continue
		}
		cv, ok := cDefines[info.Name]
		if !ok {
			t.Errorf("opcode %s missing C #define", info.Name)
			continue
		}
		if cv != byte(op) {
			t.Errorf("opcode %s: Go=0x%02x C=0x%02x", info.Name, op, cv)
		}
	}
}

// TestInterpreterCoversAllOpcodes 验证解释器 switch 覆盖每个标准 opcode。
func TestInterpreterCoversAllOpcodes(t *testing.T) {
	source, err := os.ReadFile(runtimeSourcePath(t))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	caseRe := regexp.MustCompile(`case\s+(OP_[A-Z_]+)\s*:`)
	handled := map[string]bool{}
	for _, m := range caseRe.FindAllStringSubmatch(text, -1) {
		handled[m[1]] = true
	}

	for op, info := range BaseOpcodes {
		define := "OP_" + info.Name
		if !handled[define] {
			t.Errorf("opcode %s (0x%02x) has no case in vm_execute switch — runtime would halt with 'unknown opcode'", info.Name, op)
		}
	}
}

// TestNewOpcodesPresent 锚定本次补齐的无符号比较指令。
func TestNewOpcodesPresent(t *testing.T) {
	for op, name := range map[Opcode]string{OP_CMP_ULT: "CMP_ULT", OP_CMP_UGT: "CMP_UGT", OP_CMP_ULE: "CMP_ULE", OP_CMP_UGE: "CMP_UGE"} {
		info, ok := BaseOpcodes[op]
		if !ok || info.Name != name {
			t.Fatalf("opcode 0x%02x should be %s", op, name)
		}
	}
}

func parseHex(t *testing.T, s string) byte {
	t.Helper()
	v, err := strconv.ParseInt(s, 0, 32)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return byte(v)
}
