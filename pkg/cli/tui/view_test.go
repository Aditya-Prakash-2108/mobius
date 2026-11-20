package tui

import "testing"

func TestShowStepsTraceContextCombos(t *testing.T) {
	buf := NewChatBuffer()
	buf.ApplyLiveEvent(AgentStepEvent{SourceRef: SourceRef{}, Text: "s"}, Settings{ShowSteps: false, ShowTrace: true, ShowContext: true})
	buf.ApplyLiveEvent(RouteTraceEvent{SourceRef: SourceRef{}, Text: "trace"}, Settings{ShowSteps: true, ShowTrace: false, ShowContext: true})
	if len(buf.Blocks) != 0 {
		t.Fatal("hidden kinds should not append")
	}
	s := Settings{ShowSteps: true, ShowTrace: true, ShowContext: false}
	sz := ComputeLayout(80, 24, s.ShowContext, 1, 0)
	if sz.SidebarW != 0 || sz.ChatW != 80 {
		t.Fatalf("show_context=false layout: %+v", sz)
	}
}
