package passes

import (
	"strings"
	"testing"

	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
)

func TestDumpCFGDOTEmptyModuleWellFormed(t *testing.T) {
	if llvmwrap.HasNative() {
		t.Skip("native build needs a real module; covered by pipeline_llvm_test.go")
	}
	var m llvmwrap.Module
	dot := DumpCFGDOT(&m)
	if !strings.HasPrefix(dot, "digraph cfg {") {
		t.Fatalf("missing digraph header: %q", dot)
	}
	if !strings.HasSuffix(strings.TrimSpace(dot), "}") {
		t.Fatalf("unterminated dot graph: %q", dot)
	}
}
