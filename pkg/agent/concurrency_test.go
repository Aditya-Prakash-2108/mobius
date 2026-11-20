package agent

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mobius/pkg/agentctx"
	"mobius/pkg/llm"
	"mobius/pkg/tools"
)

func chdirModuleRoot(t *testing.T) {
	t.Helper()
	start, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chdir(start) })
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("module root not found from %s", start)
		}
		dir = parent
	}
}

type concurrentMockProvider struct {
	mu    sync.Mutex
	delay time.Duration
}

func (p *concurrentMockProvider) Name() string { return "concurrent-mock" }

func (p *concurrentMockProvider) Generate(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.mu.Lock()
	d := p.delay
	p.mu.Unlock()
	if d > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(d):
		}
	}
	return &llm.ChatResponse{
		Content: "finished",
		Usage:   llm.Usage{PromptTokens: 10, CompletionTokens: 5},
	}, nil
}

func TestAgentConcurrentRunAndMutation(t *testing.T) {
	chdirModuleRoot(t)
	reg := tools.NewRegistry()
	provider := &concurrentMockProvider{delay: 2 * time.Millisecond}

	ag1, err := NewAgent(provider, reg, "model-a", 1.0, 2.0, nil)
	if err != nil {
		t.Fatalf("NewAgent ag1: %v", err)
	}
	ag2, err := NewAgent(provider, reg, "model-b", 1.0, 2.0, nil)
	if err != nil {
		t.Fatalf("NewAgent ag2: %v", err)
	}

	var hookLines atomic.Int64
	noopHook := func(string) { hookLines.Add(1) }
	ag1.SetLineHook(noopHook)
	ag2.SetLineHook(noopHook)

	ctx1 := agentctx.NewConversationContext("system-1")
	ctx2 := agentctx.NewConversationContext("system-2")

	var wg sync.WaitGroup
	var mu sync.Mutex
	var testErr error
	record := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if testErr == nil {
			testErr = err
		}
		mu.Unlock()
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		_, err := ag1.Run(context.Background(), ctx1, "goal one")
		record(err)
	}()
	go func() {
		defer wg.Done()
		_, err := ag2.Run(context.Background(), ctx2, "goal two")
		record(err)
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			ag1.SetLineHook(noopHook)
			ag2.SetLineHook(nil)
			ag1.SetModel("model-a-alt", provider, 3.0, 4.0)
			ag2.SetModel("model-b-alt", provider, 5.0, 6.0)
			_ = ag1.GetModel()
			_ = ag2.GetModel()
			_ = ag1.BudgetStatus()
			_ = ag2.BudgetStatus()
			_ = ctx1.Messages()
			_ = ctx2.Messages()
		}
	}()
	wg.Wait()

	if testErr != nil {
		t.Fatalf("concurrent run: %v", testErr)
	}
	if hookLines.Load() == 0 {
		t.Fatal("expected line hook to be invoked during Run")
	}
}
