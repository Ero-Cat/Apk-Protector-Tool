package passes

import (
	"fmt"
	"math/rand"

	"github.com/Ero-Cat/Apk-Protector-Tool/config"
	"github.com/Ero-Cat/Apk-Protector-Tool/llvmwrap"
	"github.com/Ero-Cat/Apk-Protector-Tool/report"
)

// CFFlattenPass：实现调度器式控制流平坦化，将函数的控制流图转换为单一 switch 调度循环。
type CFFlattenPass struct {
	Cfg    *config.Config
	Rand   *rand.Rand
	Report *report.Report
}

func (p *CFFlattenPass) Name() string { return "cf-flatten" }

func (p *CFFlattenPass) Run(m *llvmwrap.Module) error {
	flattenedCount := 0

	for _, fn := range m.Functions() {
		if skipFunction(fn.Name(), p.Cfg) {
			continue
		}
		blocks := fn.BasicBlocks()
		if len(blocks) < 3 {
			// 至少需要 3 个基本块才值得平坦化
			continue
		}
		if !shouldRunByRatio(p.Rand, p.Cfg.Obfuscation.FlattenRatio) {
			continue
		}

		if err := p.flattenFunction(fn); err != nil {
			p.Report.AddMessage(fmt.Sprintf("cf-flatten: skip %s: %v", fn.Name(), err))
			continue
		}

		p.Report.MarkObfuscated(fn.Name(), p.Name())
		flattenedCount++
	}

	p.Report.AddMessage(fmt.Sprintf("cf-flatten: flattened %d functions", flattenedCount))
	return nil
}

// flattenFunction 对单个函数实施控制流平坦化：分析原有终结器，把所有
// 块间边改写为"写状态变量 + 跳调度器"，调度器经 switch 分发到目标块。
// ret 终结器保持原样（函数出口）；不支持 phi/switch 等边界的函数直接跳过。
func (p *CFFlattenPass) flattenFunction(fn llvmwrap.Function) error {
	blocks := fn.BasicBlocks()
	if len(blocks) < 3 {
		return nil
	}

	// 预检：phi 的入边被改写后语义不再成立，遇到即整体跳过该函数。
	for _, block := range blocks {
		for _, inst := range block.Instructions() {
			if inst.Opcode() == "phi" {
				return fmt.Errorf("function contains phi nodes")
			}
		}
	}

	// 块 -> 随机状态值（随机化让状态不可顺序猜测）。
	stateByRef := make(map[uintptr]int64, len(blocks))
	usedStates := make(map[int64]bool, len(blocks))
	for _, block := range blocks {
		state := int64(p.Rand.Intn(0x7FFFFFFF))
		for usedStates[state] {
			state = int64(p.Rand.Intn(0x7FFFFFFF))
		}
		usedStates[state] = true
		stateByRef[block.AsValue().RefID()] = state
	}

	// 入口块开头放状态变量（alloca 插在首指令之前）。
	entryInsts := blocks[0].Instructions()
	if len(entryInsts) == 0 {
		return fmt.Errorf("entry block has no instructions")
	}
	entryBuilder := llvmwrap.NewBuilderAt(entryInsts[0])
	statePtr := entryBuilder.CreateAlloca(llvmwrap.IntType(32), "gp.state.ptr")
	entryBuilder.Dispose()

	i32 := llvmwrap.IntType(32)
	storeState := func(b llvmwrap.Builder, blockRef uintptr) {
		b.CreateStore(llvmwrap.ConstInt(stateByRef[blockRef], 32), statePtr)
	}

	// 调度器块：load 状态 + switch 分发到各原始块（default 自旋，视为损坏状态）。
	// entry 不能成为任何跳转目标（合法 IR 无入边），因此不设 entry 的 case。
	dispatcherBB := fn.AppendBasicBlock("gp.dispatcher")
	dispatchBuilder := llvmwrap.NewBuilderAtEnd(dispatcherBB)
	currentState := dispatchBuilder.CreateLoad(i32, statePtr, "gp.state")
	switchInst := dispatchBuilder.CreateSwitch(currentState, dispatcherBB, len(blocks)-1)
	for _, block := range blocks[1:] {
		switchInst.AddCase(llvmwrap.ConstInt(stateByRef[block.AsValue().RefID()], 32), block)
	}
	dispatchBuilder.Dispose()

	// 改写每个原始块的终结器：边 -> 写目标状态 + 跳调度器。
	for _, block := range blocks {
		term := block.Terminator()
		switch term.Opcode() {
		case "ret":
			// 函数出口保持原样。
			continue
		case "br":
			ops := term.Operands()
			if len(ops) == 1 {
				succ := ops[0].RefID()
				term.EraseFromParent()
				b := llvmwrap.NewBuilderAtEnd(block)
				storeState(b, succ)
				b.CreateBr(dispatcherBB)
				b.Dispose()
				continue
			}
			if len(ops) >= 3 {
				cond, tRef, fRef := ops[0], ops[1].RefID(), ops[2].RefID()
				// 条件边拆成两个"写状态 + 回调度器"跳板。
				tTramp := fn.AppendBasicBlock("gp.tramp.t")
				fTramp := fn.AppendBasicBlock("gp.tramp.f")
				tb := llvmwrap.NewBuilderAtEnd(tTramp)
				storeState(tb, tRef)
				tb.CreateBr(dispatcherBB)
				tb.Dispose()
				fb := llvmwrap.NewBuilderAtEnd(fTramp)
				storeState(fb, fRef)
				fb.CreateBr(dispatcherBB)
				fb.Dispose()

				term.EraseFromParent()
				b := llvmwrap.NewBuilderAtEnd(block)
				b.CreateCondBr(cond, tTramp, fTramp)
				b.Dispose()
				continue
			}
			return fmt.Errorf("br with %d operands", len(ops))
		default:
			return fmt.Errorf("unsupported terminator %q", term.Opcode())
		}
	}
	return nil
}
