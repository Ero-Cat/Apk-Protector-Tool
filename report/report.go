package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Report 记录每个函数被哪些 Pass 处理，以及虚拟化/消息摘要。
type Report struct {
	mu           sync.Mutex
	Instrumented map[string][]string `json:"instrumented"`
	Obfuscated   map[string][]string `json:"obfuscated"`
	Virtualized  []string            `json:"virtualized"`
	Messages     []string            `json:"messages"`
}

func New() *Report {
	return &Report{
		Instrumented: map[string][]string{},
		Obfuscated:   map[string][]string{},
		Virtualized:  []string{},
		Messages:     []string{},
	}
}

func (r *Report) MarkInstrumented(fn, pass string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Instrumented[fn] = append(r.Instrumented[fn], pass)
}

func (r *Report) MarkObfuscated(fn, pass string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Obfuscated[fn] = append(r.Obfuscated[fn], pass)
}

func (r *Report) MarkVirtualized(fn string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Virtualized = append(r.Virtualized, fn)
}

func (r *Report) AddMessage(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Messages = append(r.Messages, msg)
}

func (r *Report) Write(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
