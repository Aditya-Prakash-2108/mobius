package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportEventChatsLoadsPreviousThreads(t *testing.T) {
	chatDir := filepath.Join(t.TempDir(), "chats")
	eventsDir := filepath.Join(t.TempDir(), "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	older := `{"thread_id":"th_old","type":"user_message","content":"list the llm apis","timestamp":"2026-11-20T10:00:00Z"}
{"thread_id":"th_old","type":"tool_result","content":"ignored","timestamp":"2026-11-20T10:00:01Z"}
{"thread_id":"th_old","type":"assistant_message","content":"OpenRouter and Ollama","timestamp":"2026-11-20T10:00:02Z"}
`
	newer := `{"thread_id":"th_new","type":"user_message","content":"review the config folder","timestamp":"2026-11-20T12:00:00Z"}
{"thread_id":"th_new","type":"assistant_message","content":"The files are valid.","timestamp":"2026-11-20T12:01:00Z"}
`
	toolsOnly := `{"thread_id":"th_tools","type":"tool_result","content":"no question here","timestamp":"2026-11-20T11:00:00Z"}
`
	if err := os.WriteFile(filepath.Join(eventsDir, "old.jsonl"), []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eventsDir, "new.jsonl"), []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(eventsDir, "tools.jsonl"), []byte(toolsOnly), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureChat(chatDir, "th_empty", "default"); err != nil {
		t.Fatal(err)
	}

	if err := ImportEventChats(chatDir, eventsDir); err != nil {
		t.Fatal(err)
	}
	recs, active, err := LoadChats(chatDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("chats = %d, want the empty chat plus two event threads", len(recs))
	}
	if active != "th_new" {
		t.Fatalf("active %q, want the newest previous chat", active)
	}
	var found bool
	for _, rec := range recs {
		if rec.ID == "th_old" && len(rec.Turns) == 2 && rec.Turns[1].Text == "OpenRouter and Ollama" {
			found = true
		}
	}
	if !found {
		t.Fatalf("old chat missing: %+v", recs)
	}
	if err := ImportEventChats(chatDir, eventsDir); err != nil {
		t.Fatal(err)
	}
	again, _, err := LoadChats(chatDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 3 {
		t.Fatalf("import duplicated chats: %d", len(again))
	}
	eventFile := filepath.Join(eventsDir, "th_old.jsonl")
	if err := os.WriteFile(eventFile, []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := DeleteChat(chatDir, eventsDir, "th_old"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(chatDir, "th_old.json")); !os.IsNotExist(err) {
		t.Fatal("chat file should be removed")
	}
	if _, err := os.Stat(eventFile); !os.IsNotExist(err) {
		t.Fatal("event log should be removed")
	}
	if err := os.WriteFile(eventFile, []byte(older), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ImportEventChats(chatDir, eventsDir); err != nil {
		t.Fatal(err)
	}
	after, _, err := LoadChats(chatDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range after {
		if rec.ID == "th_old" {
			t.Fatal("deleted chat was restored from the event log")
		}
	}
}
