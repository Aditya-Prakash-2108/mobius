package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// TranscriptTurn is one saved line of a chat.
type TranscriptTurn struct {
	Role    string `json:"role"`
	Speaker string `json:"speaker,omitempty"`
	Text    string `json:"text"`
}

// ChatRecord is one chat stored on disk.
type ChatRecord struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	UpdatedAt time.Time        `json:"updated_at"`
	Turns     []TranscriptTurn `json:"turns"`
}

type chatIndex struct {
	Active  string   `json:"active"`
	Order   []string `json:"order"`
	Deleted []string `json:"deleted,omitempty"`
}

var chatMu sync.Mutex

// LoadChats reads every saved chat. A missing directory returns no chats.
func LoadChats(dir string) ([]ChatRecord, string, error) {
	chatMu.Lock()
	defer chatMu.Unlock()
	idx, err := readIndex(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", err
	}
	var out []ChatRecord
	for _, id := range idx.Order {
		rec, err := readChat(dir, id)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, "", err
		}
		out = append(out, rec)
	}
	return out, idx.Active, nil
}

// EnsureChat creates an empty chat file when that id is not stored yet.
func EnsureChat(dir, id, name string) error {
	chatMu.Lock()
	defer chatMu.Unlock()
	if _, err := readChat(dir, id); err == nil {
		return setActive(dir, id)
	}
	rec := ChatRecord{ID: id, Name: name, UpdatedAt: time.Now(), Turns: []TranscriptTurn{}}
	if err := writeChat(dir, rec); err != nil {
		return err
	}
	return setActive(dir, id)
}

// DeleteChat removes one chat from disk and remembers the id so a later
// import of .mobius/events does not restore it. eventsDir may be empty.
func DeleteChat(dir, eventsDir, id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	chatMu.Lock()
	defer chatMu.Unlock()
	if err := os.Remove(filepath.Join(dir, id+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if eventsDir != "" {
		if err := os.Remove(filepath.Join(eventsDir, id+".jsonl")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	idx, err := readIndex(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	order := make([]string, 0, len(idx.Order))
	for _, existing := range idx.Order {
		if existing != id {
			order = append(order, existing)
		}
	}
	idx.Order = order
	if !containsID(idx.Deleted, id) {
		idx.Deleted = append(idx.Deleted, id)
	}
	if idx.Active == id {
		idx.Active = ""
		if len(order) > 0 {
			idx.Active = order[0]
		}
	}
	return writeIndex(dir, idx)
}

func containsID(ids []string, id string) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

// SetActiveChat remembers which chat should open next time.
func SetActiveChat(dir, id string) error {
	chatMu.Lock()
	defer chatMu.Unlock()
	return setActive(dir, id)
}

// AppendTurn stores one user, assistant, or trace line and returns the chat name.
func AppendTurn(dir, id, name string, turn TranscriptTurn) (string, error) {
	turn.Text = strings.TrimSpace(turn.Text)
	if turn.Text == "" || id == "" {
		return name, nil
	}
	chatMu.Lock()
	defer chatMu.Unlock()
	rec, err := readChat(dir, id)
	if err != nil {
		if !os.IsNotExist(err) {
			return name, err
		}
		rec = ChatRecord{ID: id, Name: name}
	}
	if rec.Name == "" {
		rec.Name = name
	}
	if isGenericName(rec.Name) && turn.Role == "user" {
		rec.Name = titleFrom(turn.Text)
	}
	rec.Turns = append(rec.Turns, turn)
	rec.UpdatedAt = time.Now()
	if err := writeChat(dir, rec); err != nil {
		return rec.Name, err
	}
	if err := setActive(dir, id); err != nil {
		return rec.Name, err
	}
	return rec.Name, nil
}

// History returns the saved turns for one chat.
func History(dir, id string) ([]TranscriptTurn, error) {
	chatMu.Lock()
	defer chatMu.Unlock()
	rec, err := readChat(dir, id)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return rec.Turns, nil
}

func isGenericName(name string) bool {
	switch strings.TrimSpace(name) {
	case "", "default", "New Chat", "orchestrator":
		return true
	default:
		return false
	}
}

func titleFrom(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "New Chat"
	}
	if utf8.RuneCountInString(text) <= 42 {
		return text
	}
	runes := []rune(text)
	return string(runes[:42]) + "…"
}

func readIndex(dir string) (chatIndex, error) {
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return chatIndex{}, err
	}
	var idx chatIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return chatIndex{}, err
	}
	return idx, nil
}

func readChat(dir, id string) (ChatRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return ChatRecord{}, err
	}
	var rec ChatRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return ChatRecord{}, err
	}
	if rec.ID == "" {
		rec.ID = id
	}
	return rec, nil
}

func writeChat(dir string, rec ChatRecord) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, rec.ID+".json"), data, 0o644)
}

func setActive(dir, id string) error {
	idx, err := readIndex(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	found := false
	for _, existing := range idx.Order {
		if existing == id {
			found = true
			break
		}
	}
	if !found && id != "" {
		idx.Order = append(idx.Order, id)
	}
	idx.Active = id
	return writeIndex(dir, idx)
}

func writeIndex(dir string, idx chatIndex) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.json"), data, 0o644)
}
