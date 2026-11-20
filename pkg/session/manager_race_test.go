package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"mobius/pkg/agent"
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

type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) Generate(context.Context, *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: "ok"}, nil
}

func newTestAgent(t *testing.T) *agent.Agent {
	t.Helper()
	reg := tools.NewRegistry()
	ag, err := agent.NewAgent(stubProvider{}, reg, "test-model", 1, 2, nil)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return ag
}

func TestManagerConcurrentSessionOps(t *testing.T) {
	chdirModuleRoot(t)
	defaultAg := newTestAgent(t)
	mgr := NewManager(NewSession(defaultAg.ThreadID(), "default", defaultAg))

	const workers = 12
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

	for i := 0; i < workers; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			reg := tools.NewRegistry()
			ag, err := agent.NewAgent(stubProvider{}, reg, "test-model", 1, 2, nil)
			if err != nil {
				record(err)
				return
			}
			_ = mgr.CreateSession("chat", ag)
		}()
		go func() {
			defer wg.Done()
			_ = mgr.ListSession()
		}()
		go func() {
			defer wg.Done()
			list := mgr.ListSession()
			if len(list) == 0 {
				return
			}
			_, err := mgr.SwitchSession(list[0].ID)
			record(err)
		}()
	}
	wg.Wait()

	if testErr != nil {
		t.Fatalf("concurrent manager ops: %v", testErr)
	}

	list := mgr.ListSession()
	if len(list) < 1 {
		t.Fatalf("expected sessions in manager, got %d", len(list))
	}
	active, err := mgr.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	if active == nil {
		t.Fatal("expected active session")
	}
}
