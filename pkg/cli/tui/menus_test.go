package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEmptyOnlyMenuTriggers(t *testing.T) {
	if MenuTrigger("", '/') != MenuActions {
		t.Fatal("slash on empty")
	}
	if MenuTrigger("hi", '/') != MenuNone {
		t.Fatal("slash mid draft blocked")
	}
	if MenuTrigger("", '*') != MenuRoles {
		t.Fatal("star on empty")
	}
	if MenuTrigger("word", '*') != MenuNone {
		t.Fatal("star mid sentence is text")
	}
	if MenuTrigger("", '=') != MenuView {
		t.Fatal("equals on empty")
	}
}

func TestActionMenuListsEveryItem(t *testing.T) {
	m := &Model{menu: MenuActions}
	m.loadMenuItems()
	out := StripANSI(renderMenuFooter(m, 80, len(actionLabels)))
	if strings.Count(out, "\n")+1 != len(actionLabels) {
		t.Fatalf("menu lines = %d, want %d\n%s", strings.Count(out, "\n")+1, len(actionLabels), out)
	}
	for _, label := range actionLabels {
		if !strings.Contains(out, label) {
			t.Fatalf("menu missing %q\n%s", label, out)
		}
	}
}

func TestSpaceInsertsIntoDraft(t *testing.T) {
	m := &Model{store: NewChatStore("c"), chatID: "c", composeLines: 1, draft: "hi"}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.draft != "hi " {
		t.Fatalf("space dropped, draft %q", m.draft)
	}
	m.draft = ""
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if m.draft != " " {
		t.Fatalf("leading space dropped, draft %q", m.draft)
	}
}

func TestMentionFallback(t *testing.T) {
	if got := MentionFallback('x'); got != "*x" {
		t.Fatalf("got %q", got)
	}
}
