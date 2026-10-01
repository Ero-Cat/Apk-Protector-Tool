package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// CommandRunner 抽象外部二进制执行（ROADMAP P4.1）：流水线编排测试注入
// 假实现即可覆盖 scan→protect→align→sign→verify→report 全链路，CI 无
// Android 工具也能测。真实实现透传 os/exec。
type CommandRunner interface {
	// Run 流式执行：stdout/stderr 写入调用方给定的写入器（Tool 的输出
	// 通道），超时/取消语义与原 runCommand 一致。
	Run(ctx context.Context, bin string, args []string, env map[string]string,
		timeout time.Duration, stdout, stderr io.Writer) error
	// Output 执行并捕获合并输出（scanner 的验签路径）。
	Output(bin string, args []string) ([]byte, error)
	// LookPath 等价 exec.LookPath；显式路径由调用方自行 Stat。
	LookPath(bin string) (string, error)
}

// realRunner 是生产实现。
type realRunner struct{}

func (realRunner) Run(ctx context.Context, bin string, args []string, env map[string]string,
	timeout time.Duration, stdout, stderr io.Writer) error {
	if bin == "" {
		return fmt.Errorf("command path is empty")
	}
	cctx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		cctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), formatEnv(env)...)
	}
	return cmd.Run()
}

func (realRunner) Output(bin string, args []string) ([]byte, error) {
	return exec.Command(bin, args...).CombinedOutput()
}

func (realRunner) LookPath(bin string) (string, error) {
	return exec.LookPath(bin)
}
