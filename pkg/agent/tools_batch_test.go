package agent

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mobius/pkg/agentctx"
	"mobius/pkg/llm"
	"mobius/pkg/tools"
)

type slowTool struct {
	name    string
	delay   time.Duration
	started atomic.Int32
}

func (s *slowTool) Name() string        { return s.name }
func (s *slowTool) Description() string { return "slow tool for concurrency tests" }
func (s *slowTool) Schema() tools.ToolSchema {
	return tools.ToolSchema{Type: "object"}
}
func (s *slowTool) Execute(ctx context.Context, args string) (string, error) {
	s.started.Add(1)
	select {
	case <-time.After(s.delay):
		return "done:" + args, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestRunToolCalls_Parallel(t *testing.T) {
	reg := tools.NewRegistry()
	toolA := &slowTool{name: "view_file", delay: 80 * time.Millisecond}
	toolB := &slowTool{name: "grep_search", delay: 80 * time.Millisecond}
	reg.Register(toolA)
	reg.Register(toolB)

	ag := &Agent{
		threadID: "parallel-test",
		registry: reg,
		timeout:  5 * time.Second,
	}

	calls := []llm.ToolCall{
		{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "view_file", Arguments: `{"path":"a.go"}`}},
		{ID: "c2", Type: "function", Function: llm.FunctionCall{Name: "grep_search", Arguments: `{"pattern":"foo"}`}},
	}

	conv := agentctx.NewConversationContext("test")
	start := time.Now()
	modified := ag.runToolCalls(context.Background(), conv, 1, calls)
	elapsed := time.Since(start)

	if modified {
		t.Fatal("expected no file modifications")
	}
	if toolA.started.Load() != 1 || toolB.started.Load() != 1 {
		t.Fatalf("expected both tools to run, got view=%d grep=%d", toolA.started.Load(), toolB.started.Load())
	}
	if elapsed >= 130*time.Millisecond {
		t.Fatalf("expected parallel execution (~80ms), took %v", elapsed)
	}

	msgs := conv.Messages()
	toolResults := 0
	for _, m := range msgs {
		if m.Role == llm.RoleTool {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Fatalf("expected 2 tool results in context, got %d", toolResults)
	}
}

func TestRunToolCalls_StopsAtStepBudget(t *testing.T) {
	reg := tools.NewRegistry()
	tool := &slowTool{name: "view_file"}
	reg.Register(tool)

	ag := &Agent{
		threadID: "budget-test",
		registry: reg,
		maxSteps: 2,
	}
	var calls []llm.ToolCall
	for i := 0; i < 5; i++ {
		calls = append(calls, llm.ToolCall{
			ID:   fmt.Sprintf("c%d", i),
			Type: "function",
			Function: llm.FunctionCall{
				Name:      "view_file",
				Arguments: fmt.Sprintf(`{"path":"%d"}`, i),
			},
		})
	}

	conv := agentctx.NewConversationContext("test")
	ag.runToolCalls(context.Background(), conv, 1, calls)

	if tool.started.Load() != 2 {
		t.Fatalf("expected 2 tool executions under max_steps=2, got %d", tool.started.Load())
	}
	skipped := 0
	for _, m := range conv.Messages() {
		if m.Role == llm.RoleTool && strings.Contains(m.Content, "Not run") {
			skipped++
		}
	}
	if skipped != 3 {
		t.Fatalf("expected 3 skipped tool results, got %d", skipped)
	}
}
