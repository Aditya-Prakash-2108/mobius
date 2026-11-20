package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type eventLine struct {
	ThreadID  string    `json:"thread_id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ImportEventChats turns older event logs into chats when those threads were
// never saved. A chat that already has a file is left as it is.
func ImportEventChats(chatDir, eventsDir string) error {
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var found []ChatRecord
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".jsonl") {
			continue
		}
		rec, ok, err := chatFromEventFile(filepath.Join(eventsDir, ent.Name()))
		if err != nil || !ok {
			if err != nil {
				return err
			}
			continue
		}
		found = append(found, rec)
	}
	if len(found) == 0 {
		return nil
	}

	chatMu.Lock()
	defer chatMu.Unlock()
	idx, err := readIndex(chatDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	deleted := map[string]bool{}
	for _, id := range idx.Deleted {
		deleted[id] = true
	}
	var newest ChatRecord
	for _, rec := range found {
		if deleted[rec.ID] {
			continue
		}
		if _, err := readChat(chatDir, rec.ID); err == nil {
			continue
		}
		if err := writeChat(chatDir, rec); err != nil {
			return err
		}
		idx.Order = append(idx.Order, rec.ID)
		if newest.ID == "" || rec.UpdatedAt.After(newest.UpdatedAt) {
			newest = rec
		}
	}
	if newest.ID == "" {
		return nil
	}
	active, err := readChat(chatDir, idx.Active)
	if err != nil || len(active.Turns) == 0 {
		idx.Active = newest.ID
	}
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(chatDir, "index.json"), data, 0o644)
}

func chatFromEventFile(path string) (ChatRecord, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return ChatRecord{}, false, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var rec ChatRecord
	var sawUser bool
	for sc.Scan() {
		var ev eventLine
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		text := strings.TrimSpace(ev.Content)
		if ev.ThreadID != "" && rec.ID == "" {
			rec.ID = ev.ThreadID
		}
		if !ev.Timestamp.IsZero() && ev.Timestamp.After(rec.UpdatedAt) {
			rec.UpdatedAt = ev.Timestamp
		}
		switch ev.Type {
		case "user_message":
			if text == "" {
				continue
			}
			if !sawUser {
				rec.Name = titleFrom(text)
				sawUser = true
			}
			rec.Turns = append(rec.Turns, TranscriptTurn{Role: "user", Speaker: "user", Text: text})
		case "assistant_message":
			if text == "" {
				continue
			}
			rec.Turns = append(rec.Turns, TranscriptTurn{Role: "assistant", Speaker: "assistant", Text: text})
		}
	}
	if err := sc.Err(); err != nil {
		return ChatRecord{}, false, err
	}
	if !sawUser || rec.ID == "" {
		return ChatRecord{}, false, nil
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now()
	}
	return rec, true, nil
}
