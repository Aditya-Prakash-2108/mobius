package session

import "testing"

func TestChatsReloadInOrder(t *testing.T) {
	dir := t.TempDir()
	name, err := AppendTurn(dir, "th_one", "default", TranscriptTurn{Role: "user", Text: "list the llm apis"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "list the llm apis" {
		t.Fatalf("name %q", name)
	}
	if _, err := AppendTurn(dir, "th_one", name, TranscriptTurn{Role: "assistant", Speaker: "orchestrator", Text: "OpenRouter and Ollama"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendTurn(dir, "th_two", "New Chat", TranscriptTurn{Role: "user", Text: "review the config folder"}); err != nil {
		t.Fatal(err)
	}
	if err := SetActiveChat(dir, "th_one"); err != nil {
		t.Fatal(err)
	}

	recs, active, err := LoadChats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if active != "th_one" {
		t.Fatalf("active %q", active)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d chats", len(recs))
	}
	if recs[0].ID != "th_one" || recs[1].ID != "th_two" {
		t.Fatalf("order %+v %+v", recs[0].ID, recs[1].ID)
	}
	if len(recs[0].Turns) != 2 || recs[0].Turns[1].Text != "OpenRouter and Ollama" {
		t.Fatalf("turns %+v", recs[0].Turns)
	}
	if recs[1].Name != "review the config folder" {
		t.Fatalf("second name %q", recs[1].Name)
	}
	if err := DeleteChat(dir, "", "th_one"); err != nil {
		t.Fatal(err)
	}
	recs, active, err = LoadChats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].ID != "th_two" {
		t.Fatalf("after delete %+v", recs)
	}
	if active != "th_two" {
		t.Fatalf("active after delete %q", active)
	}
}
