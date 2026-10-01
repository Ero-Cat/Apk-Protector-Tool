package passes

import (
	"fmt"
	"strings"

	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
)

// DumpCFGDOT renders the control-flow graph of every defined function as a
// Graphviz DOT graph: one cluster per function, one node per basic block
// (labelled with the block name and terminator opcode), edges following the
// real terminator successors. Render with `dot -Tsvg dump-cfg-before.dot`.
func DumpCFGDOT(m *llvmwrap.Module) string {
	var b strings.Builder
	b.WriteString("digraph cfg {\n")
	b.WriteString("  node [shape=box, fontname=\"monospace\"];\n")
	b.WriteString("  rankdir=TB;\n")
	for fnIdx, fn := range m.Functions() {
		bbs := fn.BasicBlocks()
		if len(bbs) == 0 {
			continue // declaration only
		}
		fmt.Fprintf(&b, "  subgraph cluster_fn%d {\n", fnIdx)
		fmt.Fprintf(&b, "    label=\"%s\";\n", escapeDot(fn.Name()))
		b.WriteString("    style=dashed;\n")
		for i, bb := range bbs {
			label := bb.Name()
			if label == "" {
				label = fmt.Sprintf("bb%d", i)
			}
			term := bb.Terminator()
			termOp := term.Opcode()
			if termOp == "" || termOp == "unknown" {
				termOp = "?"
			}
			style := ""
			if i == 0 {
				style = ", style=filled, fillcolor=lightgrey"
			}
			fmt.Fprintf(&b, "    \"%s\" [label=\"%s\\n<%s>\"%s];\n",
				blockNodeID(fnIdx, i), escapeDot(label), termOp, style)
		}
		b.WriteString("  }\n")
		for i, bb := range bbs {
			for _, succ := range bb.Successors() {
				j := indexOfBlock(bbs, succ)
				if j < 0 {
					continue
				}
				fmt.Fprintf(&b, "  \"%s\" -> \"%s\";\n", blockNodeID(fnIdx, i), blockNodeID(fnIdx, j))
			}
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func blockNodeID(fnIdx, bbIdx int) string {
	return fmt.Sprintf("fn%d_bb%d", fnIdx, bbIdx)
}

func indexOfBlock(bbs []llvmwrap.BasicBlock, target llvmwrap.BasicBlock) int {
	id := target.AsValue().RefID()
	for i, bb := range bbs {
		if bb.AsValue().RefID() == id {
			return i
		}
	}
	return -1
}

func escapeDot(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}
