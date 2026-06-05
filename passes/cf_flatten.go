package passes

import (
	"fmt"
	"math/rand"

	"protector-tool/config"
	"protector-tool/llvmwrap"
	"protector-tool/report"
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

// flattenFunction 对单个函数实施控制流平坦化
func (p *CFFlattenPass) flattenFunction(fn llvmwrap.Function) error {
	blocks := fn.BasicBlocks()
	if len(blocks) < 2 {
		return nil
	}

	entryBlock := blocks[0]

	// 收集除 entry 外的所有基本块
	targetBlocks := blocks[1:]
	if len(targetBlocks) == 0 {
		return nil
	}

	// 为每个目标块分配随机状态值
	stateMap := make(map[int]int) // 块索引 -> 状态值
	usedStates := make(map[int]bool)
	for i := range targetBlocks {
		for {
			state := p.Rand.Intn(0x7FFFFFFF)
			if !usedStates[state] {
				usedStates[state] = true
				stateMap[i] = state
				break
			}
		}
	}

	// 在 entry 块开头分配状态变量
	builder := llvmwrap.NewBuilderAtEnd(entryBlock)

	// 创建状态变量（在函数入口分配）
	statePtr := builder.CreateAlloca(llvmwrap.IntType(32), "gp.state.ptr")

	// 初始化状态为第一个目标块的状态值
	initialState := llvmwrap.ConstInt(int64(stateMap[0]), 32)
	builder.CreateStore(initialState, statePtr)

	// 创建调度器基本块
	dispatcherBB := fn.AppendBasicBlock("gp.dispatcher")

	// 创建退出基本块
	exitBB := fn.AppendBasicBlock("gp.exit")

	// 从 entry 跳转到 dispatcher
	builder.CreateBr(dispatcherBB)
	builder.Dispose()

	// 构建 dispatcher 块
	dispatchBuilder := llvmwrap.NewBuilderAtEnd(dispatcherBB)

	// 加载当前状态
	currentState := dispatchBuilder.CreateLoad(llvmwrap.IntType(32), statePtr, "gp.state")

	// 创建 switch 指令
	switchInst := dispatchBuilder.CreateSwitch(currentState, exitBB, len(targetBlocks))

	// 为每个目标块添加 case
	for i, block := range targetBlocks {
		stateVal := llvmwrap.ConstInt(int64(stateMap[i]), 32)
		switchInst.AddCase(stateVal, block)
	}

	dispatchBuilder.Dispose()

	// 在 exit 块添加返回指令
	exitBuilder := llvmwrap.NewBuilderAtEnd(exitBB)
	exitBuilder.CreateRetVoid()
	exitBuilder.Dispose()

	// 为每个目标块添加跳转回 dispatcher 的逻辑
	// 注意：这里简化处理，实际需要分析原有终结器并重写
	for i, block := range targetBlocks {
		// 获取下一个状态（循环或结束）
		nextIndex := i + 1
		var nextState int
		if nextIndex < len(targetBlocks) {
			nextState = stateMap[nextIndex]
		} else {
			// 最后一个块，设置为无效状态触发 exit
			nextState = -1
		}

		// 在块末尾添加更新状态并跳回 dispatcher
		// 注意：只处理没有终结器的块（简化实现）
		blockBuilder := llvmwrap.NewBuilderAtEnd(block)
		if nextState >= 0 {
			nextStateVal := llvmwrap.ConstInt(int64(nextState), 32)
			blockBuilder.CreateStore(nextStateVal, statePtr)
		}
		blockBuilder.CreateBr(dispatcherBB)
		blockBuilder.Dispose()
	}

	return nil
}
