package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Connection is an API endpoint entered in the app, instead of a .env file.
// The API key is held in memory and in the system keystore, not in the config file.
type Connection struct {
	ID              string
	Name            string
	Protocol        string
	BaseURL         string
	APIKey          string
	Model           string
	PromptPrice     float64
	CompletionPrice float64
	HasPrice        bool
}

// APISummary is one saved endpoint without its key.
type APISummary struct {
	ID              string
	Name            string
	BaseURL         string
	Model           string
	Protocol        string
	Current         bool
	HasKey          bool
	HasPrice        bool
	PromptPrice     string
	CompletionPrice string
}

// ConnectionForm is the editable view of a connection. The API key is never included.
type ConnectionForm struct {
	BaseURL         string
	Model           string
	HasKey          bool
	HasPrice        bool
	PromptPrice     string
	CompletionPrice string
}

type connectionFile struct {
	ID              string   `toml:"id"`
	Name            string   `toml:"name,omitempty"`
	Protocol        string   `toml:"protocol"`
	BaseURL         string   `toml:"base_url"`
	APIKey          string   `toml:"api_key,omitempty"`
	Model           string   `toml:"model"`
	PromptPrice     *float64 `toml:"prompt_price,omitempty"`
	CompletionPrice *float64 `toml:"completion_price,omitempty"`
}

type connectionsFile struct {
	Active      string           `toml:"active"`
	Connections []connectionFile `toml:"connections"`
}

// ConnectionPath is ~/.config/mobius/connections.toml.
// API keys are not stored in that file.
func ConnectionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".config", "mobius", "connections.toml"), nil
}

// ParseConnection checks a URL, key, and model. Blank price fields mean no price.
func ParseConnection(baseURL, apiKey, model, promptRaw, completionRaw string) (Connection, error) {
	conn := Connection{
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:   strings.TrimSpace(apiKey),
		Model:    strings.TrimSpace(model),
		Protocol: ProtocolForURL(baseURL),
	}
	conn.ID = ConnectionID(conn.BaseURL, conn.Model)
	if conn.BaseURL == "" {
		return Connection{}, fmt.Errorf("api url is required")
	}
	if !strings.HasPrefix(conn.BaseURL, "http://") && !strings.HasPrefix(conn.BaseURL, "https://") {
		return Connection{}, fmt.Errorf("api url must start with http:// or https://")
	}
	if conn.APIKey == "" {
		return Connection{}, fmt.Errorf("api key is required")
	}
	if conn.Model == "" {
		return Connection{}, fmt.Errorf("model is required")
	}
	prompt, promptSet, err := parseOptionalPrice(promptRaw)
	if err != nil {
		return Connection{}, fmt.Errorf("prompt price: %w", err)
	}
	completion, completionSet, err := parseOptionalPrice(completionRaw)
	if err != nil {
		return Connection{}, fmt.Errorf("completion price: %w", err)
	}
	if promptSet || completionSet {
		conn.HasPrice = true
		conn.PromptPrice = prompt
		conn.CompletionPrice = completion
	}
	return conn, nil
}

func parseOptionalPrice(raw string) (float64, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 0, false, fmt.Errorf("must be a number, or left blank")
	}
	return v, true, nil
}

// ConnectionID is stable for a URL and model, so saving the same pair updates it.
func ConnectionID(baseURL, model string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	sum := sha256.Sum256([]byte(baseURL + "\n" + model))
	return hex.EncodeToString(sum[:8])
}

// ProtocolForURL picks the request format. Anthropic has its own API.
// Every other URL uses the OpenAI-compatible chat format.
func ProtocolForURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	host := ""
	if err == nil {
		host = strings.ToLower(parsed.Host)
	}
	if strings.Contains(host, "anthropic.com") {
		return "anthropic"
	}
	return "openai"
}

// LoadConnection reads the active saved connection. A missing file returns an error.
func LoadConnection(path string) (Connection, error) {
	conns, active, err := LoadSaved(path)
	if err != nil {
		return Connection{}, err
	}
	for _, conn := range conns {
		if conn.ID == active {
			return conn, nil
		}
	}
	if len(conns) == 0 {
		return Connection{}, os.ErrNotExist
	}
	return conns[len(conns)-1], nil
}

// LoadSaved reads every saved API. Keys come from the system keystore.
func LoadSaved(path string) ([]Connection, string, error) {
	path, err := resolveConnectionPath(path)
	if err != nil {
		return nil, "", err
	}
	if err := migrateLegacyFile(path); err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	var file connectionsFile
	if err := toml.Unmarshal(data, &file); err != nil {
		return nil, "", fmt.Errorf("parse connections: %w", err)
	}
	var out []Connection
	dirty := false
	for _, item := range file.Connections {
		if strings.TrimSpace(item.APIKey) != "" {
			dirty = true
		}
		conn, err := connectionFromFile(item)
		if err != nil {
			return nil, "", err
		}
		out = append(out, conn)
	}
	if dirty {
		_ = writeConnectionFile(path, file)
	}
	return out, file.Active, nil
}

// SaveConnection adds or updates one API and stores its key in the system keystore.
func SaveConnection(path string, conn Connection) error {
	if conn.ID == "" {
		conn.ID = ConnectionID(conn.BaseURL, conn.Model)
	}
	if conn.Protocol == "" {
		conn.Protocol = ProtocolForURL(conn.BaseURL)
	}
	path, err := resolveConnectionPath(path)
	if err != nil {
		return err
	}
	if err := migrateLegacyFile(path); err != nil {
		return err
	}
	if err := putAPIKey(conn.ID, conn.APIKey); err != nil {
		return fmt.Errorf("save api key in the system keystore: %w", err)
	}
	existing, err := readConnectionFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	replaced := false
	for i := range existing.Connections {
		if existing.Connections[i].ID == conn.ID {
			existing.Connections[i] = fileRecord(conn)
			replaced = true
			break
		}
	}
	if conn.Name != "" {
		base := strings.TrimRight(conn.BaseURL, "/")
		for i := range existing.Connections {
			if strings.TrimRight(existing.Connections[i].BaseURL, "/") == base {
				existing.Connections[i].Name = conn.Name
			}
		}
	}
	if !replaced {
		existing.Connections = append(existing.Connections, fileRecord(conn))
	}
	existing.Active = conn.ID
	return writeConnectionFile(path, existing)
}

// UseConnection makes this endpoint active and keeps every other saved API.
func (c *Config) UseConnection(conn Connection) error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(conn.BaseURL) == "" || strings.TrimSpace(conn.Model) == "" {
		return fmt.Errorf("api url and model are required")
	}
	if conn.ID == "" {
		conn.ID = ConnectionID(conn.BaseURL, conn.Model)
	}
	if conn.Protocol == "" {
		conn.Protocol = ProtocolForURL(conn.BaseURL)
	}
	if strings.TrimSpace(conn.APIKey) == "" {
		return fmt.Errorf("api key is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.upsert(conn)
	if conn.Name != "" {
		for i := range c.saved {
			if c.saved[i].BaseURL == conn.BaseURL {
				c.saved[i].Name = conn.Name
			}
		}
	}
	copied := conn
	c.manual = &copied
	c.ActiveModel = copied.Model
	return nil
}

// ImportConnections loads saved APIs. The active id is used when its key is present.
func (c *Config) ImportConnections(conns []Connection, activeID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saved = append([]Connection(nil), conns...)
	for _, conn := range c.saved {
		if conn.ID == activeID && strings.TrimSpace(conn.APIKey) != "" {
			copied := conn
			c.manual = &copied
			c.ActiveModel = copied.Model
			return
		}
	}
}

// KeyFor returns the key already saved for this URL and model.
func (c *Config) KeyFor(id string) string {
	if c == nil || id == "" {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, conn := range c.saved {
		if conn.ID == id && conn.APIKey != "" {
			return conn.APIKey
		}
	}
	return ""
}

// SavedAPIs lists endpoints without revealing keys.
func (c *Config) SavedAPIs() []APISummary {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]APISummary, 0, len(c.saved))
	active := ""
	if c.manual != nil {
		active = c.manual.ID
	}
	for _, conn := range c.saved {
		item := APISummary{
			ID:       conn.ID,
			Name:     conn.Name,
			BaseURL:  conn.BaseURL,
			Model:    conn.Model,
			Protocol: conn.Protocol,
			Current:  conn.ID == active,
			HasKey:   conn.APIKey != "",
			HasPrice: conn.HasPrice,
		}
		if conn.HasPrice {
			item.PromptPrice = formatPrice(conn.PromptPrice)
			item.CompletionPrice = formatPrice(conn.CompletionPrice)
		}
		out = append(out, item)
	}
	return out
}

func (c *Config) upsert(conn Connection) {
	for i := range c.saved {
		if c.saved[i].ID == conn.ID {
			c.saved[i] = conn
			return
		}
	}
	c.saved = append(c.saved, conn)
}

func (c *Config) matchSaved(modelName string) (Connection, bool) {
	if c.manual != nil && (modelName == "" || c.manual.Model == modelName) {
		return *c.manual, true
	}
	var found Connection
	ok := false
	for _, conn := range c.saved {
		if conn.Model == modelName {
			found = conn
			ok = true
		}
	}
	return found, ok
}

// CurrentAPIKey returns the key entered for the active connection, or the env key for the active model.
func (c *Config) CurrentAPIKey() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.manual != nil && c.manual.APIKey != "" {
		return c.manual.APIKey
	}
	for _, prov := range c.Providers {
		for _, model := range prov.Models {
			if model == c.ActiveModel && prov.EnvKey != "" {
				if key := os.Getenv(prov.EnvKey); key != "" {
					return key
				}
			}
		}
	}
	return ""
}

// ConnectionForm returns the fields to show in the connect screen.
func (c *Config) ConnectionForm() ConnectionForm {
	if c == nil {
		return ConnectionForm{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	form := ConnectionForm{Model: c.ActiveModel}
	if c.manual != nil {
		form.BaseURL = c.manual.BaseURL
		form.Model = c.manual.Model
		form.HasKey = c.manual.APIKey != ""
		form.HasPrice = c.manual.HasPrice
		if c.manual.HasPrice {
			form.PromptPrice = formatPrice(c.manual.PromptPrice)
			form.CompletionPrice = formatPrice(c.manual.CompletionPrice)
		}
		return form
	}
	for _, prov := range c.Providers {
		for _, model := range prov.Models {
			if model == c.ActiveModel {
				form.BaseURL = prov.BaseURL
				form.HasKey = prov.EnvKey != "" && os.Getenv(prov.EnvKey) != ""
			}
		}
	}
	if c.pricesKnown(c.ActiveModel) {
		form.HasPrice = true
		form.PromptPrice = formatPrice(c.Pricing.Prompt[c.ActiveModel])
		form.CompletionPrice = formatPrice(c.Pricing.Completion[c.ActiveModel])
	}
	return form
}

func formatPrice(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func resolveConnectionPath(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return path, nil
	}
	return ConnectionPath()
}

func readConnectionFile(path string) (connectionsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return connectionsFile{}, err
	}
	var file connectionsFile
	if err := toml.Unmarshal(data, &file); err != nil {
		return connectionsFile{}, fmt.Errorf("parse connections: %w", err)
	}
	return file, nil
}

func writeConnectionFile(path string, file connectionsFile) error {
	for i := range file.Connections {
		file.Connections[i].APIKey = ""
	}
	data, err := toml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode connections: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create connection directory: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func fileRecord(conn Connection) connectionFile {
	item := connectionFile{
		ID:       conn.ID,
		Name:     conn.Name,
		Protocol: conn.Protocol,
		BaseURL:  conn.BaseURL,
		Model:    conn.Model,
	}
	if conn.HasPrice {
		prompt := conn.PromptPrice
		completion := conn.CompletionPrice
		item.PromptPrice = &prompt
		item.CompletionPrice = &completion
	}
	return item
}

func connectionFromFile(item connectionFile) (Connection, error) {
	conn := Connection{
		ID:       item.ID,
		Name:     strings.TrimSpace(item.Name),
		Protocol: item.Protocol,
		BaseURL:  strings.TrimRight(strings.TrimSpace(item.BaseURL), "/"),
		Model:    strings.TrimSpace(item.Model),
		APIKey:   strings.TrimSpace(item.APIKey),
	}
	if conn.Protocol == "" {
		conn.Protocol = ProtocolForURL(conn.BaseURL)
	}
	if conn.ID == "" {
		conn.ID = ConnectionID(conn.BaseURL, conn.Model)
	}
	if conn.APIKey != "" {
		if err := putAPIKey(conn.ID, conn.APIKey); err != nil {
			return Connection{}, fmt.Errorf("move api key into the system keystore: %w", err)
		}
	} else {
		key, err := getAPIKey(conn.ID)
		if err != nil {
			return Connection{}, fmt.Errorf("read api key from the system keystore: %w", err)
		}
		conn.APIKey = key
	}
	if item.PromptPrice != nil || item.CompletionPrice != nil {
		conn.HasPrice = true
		if item.PromptPrice != nil {
			conn.PromptPrice = *item.PromptPrice
		}
		if item.CompletionPrice != nil {
			conn.CompletionPrice = *item.CompletionPrice
		}
	}
	return conn, nil
}

func migrateLegacyFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	legacy := filepath.Join(filepath.Dir(path), "connection.toml")
	if legacy == path {
		return nil
	}
	data, err := os.ReadFile(legacy)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var old connectionFile
	if err := toml.Unmarshal(data, &old); err != nil {
		return fmt.Errorf("parse saved connection: %w", err)
	}
	if strings.TrimSpace(old.BaseURL) == "" || strings.TrimSpace(old.Model) == "" {
		return nil
	}
	conn, err := connectionFromFile(old)
	if err != nil {
		return err
	}
	if err := writeConnectionFile(path, connectionsFile{Active: conn.ID, Connections: []connectionFile{fileRecord(conn)}}); err != nil {
		return err
	}
	return os.Remove(legacy)
}
