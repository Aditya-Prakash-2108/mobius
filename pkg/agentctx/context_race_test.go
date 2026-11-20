package agentctx

import (
	"mobius/pkg/llm"
	"sync"
	"testing"
)

func TestConversationContextConcurrentAddAndRead(t *testing.T) {
	ctx := NewConversationContext("")
	const goroutines = 20
	const perG = 25

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				ctx.AddUserMessage("msg")
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < perG; j++ {
				_ = ctx.Messages()
			}
		}()
	}
	wg.Wait()

	msgs := ctx.Messages()
	if len(msgs) != goroutines*perG {
		t.Fatalf("message count = %d, want %d", len(msgs), goroutines*perG)
	}
}

func TestConversationContextMessagesCopyIsolated(t *testing.T) {
	tc := []llm.ToolCall{{ID: "id1", Type: "function", Function: llm.FunctionCall{Name: "tool", Arguments: `{}`}}}
	ctx := NewConversationContext("")
	ctx.AddAssistantMessage("assistant", tc)

	copy := ctx.Messages()
	copy[0].Content = "mutated"
	copy[0].ToolCalls[0].ID = "mutated-id"

	stored := ctx.Messages()
	if stored[0].Content != "assistant" {
		t.Fatalf("stored content mutated: %q", stored[0].Content)
	}
	if len(stored[0].ToolCalls) == 0 || stored[0].ToolCalls[0].ID != "id1" {
		t.Fatalf("stored tool call mutated: %+v", stored[0].ToolCalls)
	}
}
