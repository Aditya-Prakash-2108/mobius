package tui

import (
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestBudgetNotInTranscript(t *testing.T) {
	buf := NewChatBuffer()
	settings := DefaultSettings()
	ledger := NewBudgetLedger()
	ev := BudgetEvent{SourceRef: SourceRef{ThreadID: "a"}, InTokens: 1, OutTokens: 2, CostUSD: 0.01}
	ledger.Apply(ev)
	buf.ApplyLiveEvent(AgentStepEvent{SourceRef: SourceRef{Name: "w"}, Text: "step"}, settings)
	if buf.HasBudgetInTranscript() {
		t.Fatal("budget must not be in transcript blocks")
	}
}

func TestBudgetCumulativeReplaceAndTotal(t *testing.T) {
	l := NewBudgetLedger()
	l.Apply(BudgetEvent{SourceRef: SourceRef{ThreadID: "a"}, InTokens: 10, OutTokens: 1, CostUSD: 0.1})
	l.Apply(BudgetEvent{SourceRef: SourceRef{ThreadID: "a"}, InTokens: 20, OutTokens: 2, CostUSD: 0.2})
	tot := l.Total()
	if tot.InTokens != 20 || tot.OutTokens != 2 {
		t.Fatalf("replace per thread: %+v", tot)
	}
	l.Apply(BudgetEvent{SourceRef: SourceRef{ThreadID: "b"}, InTokens: 5, OutTokens: 5, CostUSD: 0.05})
	tot = l.Total()
	if tot.InTokens != 25 || tot.CostUSD < 0.249 {
		t.Fatalf("multi-agent sum: %+v", tot)
	}
}

func TestThoughtPromotionNoDuplicate(t *testing.T) {
	buf := NewChatBuffer()
	settings := DefaultSettings()
	buf.BeginRun()
	text := "final answer"
	buf.ApplyLiveEvent(AgentThoughtEvent{SourceRef: SourceRef{Name: "eng"}, Text: text}, settings)
	buf.ResolveFinal("Software Engineer", "software_engineer", text, "run-1", nil)
	vis := buf.VisibleBlocks()
	finals := 0
	for _, b := range vis {
		if b.Kind == BlockFinal {
			finals++
		}
		if b.Kind == BlockThought {
			t.Fatal("thought should be promoted")
		}
	}
	if finals != 1 {
		t.Fatalf("want 1 final, got %d", finals)
	}
}

func TestToolAfterThoughtMarksIntermediate(t *testing.T) {
	buf := NewChatBuffer()
	settings := DefaultSettings()
	buf.ApplyLiveEvent(AgentThoughtEvent{SourceRef: SourceRef{Name: "Software Engineer"}, Text: "thinking out loud"}, settings)
	buf.ApplyLiveEvent(AgentToolEvent{SourceRef: SourceRef{Name: "Software Engineer"}, Name: "view_file", Args: `{"path":"main.go"}`}, settings)
	for _, b := range buf.Blocks {
		if b.Kind == BlockThought || strings.Contains(b.Text, "thinking") {
			t.Fatal("thinking text should not stay in the transcript")
		}
	}
	lipgloss.SetColorProfile(termenv.TrueColor)
	lines := flattenTranscript(buf.VisibleBlocks(), 80, true)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Software Engineer") || !strings.Contains(joined, "view_file") {
		t.Fatalf("activity should name the agent and the tool:\n%s", joined)
	}
	if !strings.Contains(joined, "\x1b[3") {
		t.Fatalf("tool activity should be faded italic, got %q", joined)
	}
	reply := flattenTranscript([]TranscriptBlock{{
		Kind: BlockFinal, Speaker: "orchestrator", Text: "Hello",
	}}, 80, true)
	replyLine := ""
	for _, ln := range reply {
		if strings.Contains(ln, "Hello") {
			replyLine = ln
		}
	}
	if strings.Contains(replyLine, "\x1b[3") {
		t.Fatalf("conversation reply should stay full contrast, got %q", replyLine)
	}
}

func TestPerChatIsolation(t *testing.T) {
	store := NewChatStore("c1")
	settings := DefaultSettings()
	store.Ensure("c2")
	store.Buffer("c2").AppendUser("other", "r1")
	store.Buffer("c1").AppendUser("here", "r1")
	if len(store.Buffer("c2").Blocks) != 1 || store.Buffer("c2").Blocks[0].Text != "other" {
		t.Fatal("isolation failed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			store.Buffer("c2").ApplyLiveEvent(AgentStepEvent{SourceRef: SourceRef{}, Text: "s"}, settings)
		}(i)
	}
	wg.Wait()
	if len(store.Buffer("c1").Blocks) != 1 {
		t.Fatal("inactive chat mutated")
	}
}

func TestToolFoldingCtrlO(t *testing.T) {
	buf := NewChatBuffer()
	settings := DefaultSettings()
	buf.ApplyLiveEvent(AgentToolEvent{SourceRef: SourceRef{}, Name: "a", Args: ""}, settings)
	buf.ApplyLiveEvent(AgentToolEvent{SourceRef: SourceRef{}, Name: "b", Args: ""}, settings)
	if len(buf.Blocks) != 1 || buf.Blocks[0].ToolCount != 2 {
		t.Fatalf("expected folded group, blocks=%d", len(buf.Blocks))
	}
	buf.ToggleToolExpand()
	if !buf.Blocks[0].Expanded {
		t.Fatal("ctrl+o should expand group")
	}
}
