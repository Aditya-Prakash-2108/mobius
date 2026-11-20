package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mobius/pkg/agentctx"
	"mobius/pkg/llm"
	"mobius/pkg/tools"
)

type blockingMockProvider struct {
	mu            sync.Mutex
	generateCalls atomic.Int32
	blockCh       chan struct{}
	releaseCh     chan struct{}
	toolTurn      bool
}

func (p *blockingMockProvider) Name() string { return "blocking-mock" }

func (p *blockingMockProvider) Generate(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	p.generateCalls.Add(1)
	n := int(p.generateCalls.Load())

	p.mu.Lock()
	block := p.blockCh
	release := p.releaseCh
	toolFirst := p.toolTurn
	p.mu.Unlock()

	if n == 1 && block != nil && !toolFirst {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
	}

	if n == 1 && toolFirst {
		return &llm.ChatResponse{
			Content: "calling tool",
			ToolCalls: []llm.ToolCall{{
				ID:   "c1",
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "view_file",
					Arguments: `{"path":"README.md"}`,
				},
			}},
			Usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1},
		}, nil
	}

	if toolFirst && n == 2 && block != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
		}
	}

	return &llm.ChatResponse{
		Content: "done",
		Usage:   llm.Usage{PromptTokens: 1, CompletionTokens: 1},
	}, nil
}

func TestNoteDuringZeroToolTurn(t *testing.T) {
	chdirModuleRoot(t)
	reg := tools.NewRegistry()
	reg.Register(&dummyTool{name: "view_file"})

	provider := &blockingMockProvider{
		blockCh:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}

	ag, err := NewAgent(provider, reg, "m", 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag.SetLineHook(func(string) {})

	ctx := agentctx.NewConversationContext("sys")
	var mu sync.Mutex
	var testErr error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if testErr == nil {
			testErr = err
		}
		mu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := ag.Run(context.Background(), ctx, "goal")
		recordErr(err)
	}()

	deadline := time.After(2 * time.Second)
	for provider.generateCalls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for first Generate")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}

	const injected = "NOTE_INJECT_ZERO_TOOL"
	if !ag.PushNote(injected) {
		t.Fatal("PushNote rejected")
	}
	close(provider.releaseCh)

	wg.Wait()
	if testErr != nil {
		t.Fatalf("Run: %v", testErr)
	}

	msgs := ctx.Messages()
	var sawAssistant, sawNote bool
	for i, m := range msgs {
		if m.Role == llm.RoleAssistant && m.Content == "done" {
			sawAssistant = true
		}
		if m.Role == llm.RoleUser && m.Content == injected {
			if !sawAssistant {
				t.Fatalf("note appeared before terminal assistant at index %d", i)
			}
			sawNote = true
		}
	}
	if !sawNote {
		t.Fatal("injected note missing from context")
	}
	if provider.generateCalls.Load() < 2 {
		t.Fatalf("expected second Generate after note drain, got %d", provider.generateCalls.Load())
	}
}

func TestNoteDuringToolTurn(t *testing.T) {
	chdirModuleRoot(t)
	reg := tools.NewRegistry()
	reg.Register(&dummyTool{name: "view_file"})

	provider := &blockingMockProvider{
		blockCh:   make(chan struct{}),
		releaseCh: make(chan struct{}),
		toolTurn:  true,
	}

	ag, err := NewAgent(provider, reg, "m", 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag.SetLineHook(func(string) {})

	ctx := agentctx.NewConversationContext("sys")

	var wg sync.WaitGroup
	var runErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, runErr = ag.Run(context.Background(), ctx, "goal")
	}()

	deadline := time.After(2 * time.Second)
	for provider.generateCalls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for second Generate (after tool)")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}

	const injected = "NOTE_AFTER_TOOL"
	ag.PushNote(injected)
	close(provider.releaseCh)
	wg.Wait()
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	msgs := ctx.Messages()
	var toolResultIdx, noteIdx = -1, -1
	for i, m := range msgs {
		if m.Role == llm.RoleTool {
			toolResultIdx = i
		}
		if m.Role == llm.RoleUser && m.Content == injected {
			noteIdx = i
		}
	}
	if toolResultIdx < 0 || noteIdx < 0 {
		t.Fatalf("missing tool result or note (tool=%d note=%d)", toolResultIdx, noteIdx)
	}
	if noteIdx < toolResultIdx {
		t.Fatalf("note at %d must be after tool result at %d", noteIdx, toolResultIdx)
	}
	for i := toolResultIdx + 1; i < noteIdx; i++ {
		if msgs[i].Role == llm.RoleUser && msgs[i].Content == injected {
			continue
		}
		if msgs[i].Role == llm.RoleAssistant && len(msgs[i].ToolCalls) > 0 {
			t.Fatalf("user note must not appear between assistant tool_calls and tool result")
		}
	}
}

func TestConcurrentPushNoteDrainedOnce(t *testing.T) {
	chdirModuleRoot(t)
	reg := tools.NewRegistry()
	provider := &blockingMockProvider{
		blockCh:   make(chan struct{}),
		releaseCh: make(chan struct{}),
	}
	ag, err := NewAgent(provider, reg, "m", 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ag.SetLineHook(func(string) {})

	ctx := agentctx.NewConversationContext("sys")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = ag.Run(context.Background(), ctx, "goal")
	}()

	for provider.generateCalls.Load() < 1 {
		time.Sleep(2 * time.Millisecond)
	}

	const n = 50
	var noteWg sync.WaitGroup
	noteWg.Add(n)
	var pushed atomic.Int32
	for i := 0; i < n; i++ {
		go func(i int) {
			defer noteWg.Done()
			note := "note-" + strings.Repeat("x", i%5)
			if ag.PushNote(note) {
				pushed.Add(1)
			}
		}(i)
	}
	noteWg.Wait()
	close(provider.releaseCh)
	wg.Wait()

	count := 0
	for _, m := range ctx.Messages() {
		if m.Role == llm.RoleUser && strings.HasPrefix(m.Content, "note-") {
			count++
		}
	}
	if int(pushed.Load()) != count {
		t.Fatalf("expected %d drained notes, got %d in context", pushed.Load(), count)
	}
}
