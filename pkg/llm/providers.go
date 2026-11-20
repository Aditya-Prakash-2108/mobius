package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ProviderGroup is one API endpoint and the models that use it.
type ProviderGroup struct {
	ID      string
	Label   string
	BaseURL string
	Models  []string
}

// ProviderGroups lists the APIs the user added, one row per URL.
func (c *Config) ProviderGroups() []ProviderGroup {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []ProviderGroup
	seenURL := map[string]bool{}
	order := []string{}
	models := map[string][]string{}
	names := map[string]string{}
	for _, conn := range c.saved {
		if conn.BaseURL == "" {
			continue
		}
		if !seenURL[conn.BaseURL] {
			seenURL[conn.BaseURL] = true
			order = append(order, conn.BaseURL)
		}
		if conn.Name != "" && names[conn.BaseURL] == "" {
			names[conn.BaseURL] = conn.Name
		}
		if conn.Model != "" && !contains(models[conn.BaseURL], conn.Model) {
			models[conn.BaseURL] = append(models[conn.BaseURL], conn.Model)
		}
	}
	for _, base := range order {
		label := names[base]
		if label == "" {
			label = base
		} else {
			label += " · " + base
		}
		out = append(out, ProviderGroup{
			ID:      "url:" + base,
			Label:   label,
			BaseURL: base,
			Models:  models[base],
		})
	}
	return out
}

// AddSavedModel attaches a model to an API the user already added.
// Blank prices leave the cost hidden for that model.
func (c *Config) AddSavedModel(baseURL, model, promptRaw, completionRaw string) (Connection, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	model = strings.TrimSpace(model)
	if baseURL == "" || model == "" {
		return Connection{}, fmt.Errorf("api url and model are required")
	}
	c.mu.RLock()
	var src *Connection
	for i := range c.saved {
		if c.saved[i].BaseURL == baseURL && c.saved[i].APIKey != "" {
			copy := c.saved[i]
			src = &copy
			break
		}
	}
	c.mu.RUnlock()
	if src == nil {
		return Connection{}, fmt.Errorf("that API has no saved key")
	}
	conn, err := ParseConnection(baseURL, src.APIKey, model, promptRaw, completionRaw)
	if err != nil {
		return Connection{}, err
	}
	conn.Name = src.Name
	return conn, nil
}

// AddConfigModel remembers an extra model on a configured provider.
func (c *Config) AddConfigModel(providerKey, model, path string) error {
	providerKey = strings.TrimPrefix(providerKey, "cfg:")
	model = strings.TrimSpace(model)
	if providerKey == "" || model == "" {
		return fmt.Errorf("provider and model are required")
	}
	c.mu.Lock()
	prov, ok := c.Providers[providerKey]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("unknown API")
	}
	if !contains(prov.Models, model) {
		prov.Models = append(prov.Models, model)
		c.Providers[providerKey] = prov
	}
	c.mu.Unlock()
	return saveExtraModel(path, providerKey, model)
}

// NoteActive records the model in use. A configured provider clears a saved API
// so the next request uses that provider instead of the previous key.
func (c *Config) NoteActive(model string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ActiveModel = model
	if c.manual != nil && c.manual.Model != model {
		c.manual = nil
	}
}

// ActiveEndpoint is the saved API currently in use. The URL is empty when the
// active model comes from the configured provider list.
func (c *Config) ActiveEndpoint() (baseURL, model string) {
	if c == nil {
		return "", ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.manual != nil {
		return c.manual.BaseURL, c.manual.Model
	}
	return "", c.ActiveModel
}

// OpenGroupModel builds the client for one model on one API.
func (c *Config) OpenGroupModel(groupID, model string) (Provider, float64, float64, error) {
	if c == nil {
		return nil, 0, 0, fmt.Errorf("config is nil")
	}
	model = strings.TrimSpace(model)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if strings.HasPrefix(groupID, "url:") {
		base := strings.TrimPrefix(groupID, "url:")
		for _, conn := range c.saved {
			if conn.BaseURL == base && conn.Model == model {
				if strings.TrimSpace(conn.APIKey) == "" {
					return nil, 0, 0, fmt.Errorf("api key for %s is not in the system keystore", model)
				}
				protocol := conn.Protocol
				if protocol == "" {
					protocol = "openai"
				}
				factory, ok := providerFactories[protocol]
				if !ok {
					return nil, 0, 0, fmt.Errorf("unsupported API protocol %q for %s", protocol, base)
				}
				prompt, completion := 0.0, 0.0
				if conn.HasPrice {
					prompt, completion = conn.PromptPrice, conn.CompletionPrice
				}
				return factory(conn.APIKey, conn.BaseURL), prompt, completion, nil
			}
		}
		return nil, 0, 0, fmt.Errorf("model %q is not on %s", model, base)
	}
	key := strings.TrimPrefix(groupID, "cfg:")
	prov, ok := c.Providers[key]
	if !ok {
		return nil, 0, 0, fmt.Errorf("unknown API")
	}
	if !contains(prov.Models, model) {
		return nil, 0, 0, fmt.Errorf("model %q is not on %s", model, prov.Name)
	}
	factory, ok := providerFactories[prov.Type]
	if !ok {
		return nil, 0, 0, fmt.Errorf("unsupported API protocol %q for %s", prov.Type, prov.BaseURL)
	}
	return factory(os.Getenv(prov.EnvKey), prov.BaseURL), c.Pricing.Prompt[model], c.Pricing.Completion[model], nil
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

type extraModelsFile struct {
	Providers map[string]extraProvider `toml:"providers"`
}

type extraProvider struct {
	Models []string `toml:"models"`
}

func extraModelsPath(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mobius", "extra_models.toml"), nil
}

// LoadExtraModels appends models the user added onto configured providers.
func LoadExtraModels(c *Config, path string) error {
	if c == nil {
		return nil
	}
	path, err := extraModelsPath(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file extraModelsFile
	if err := toml.Unmarshal(data, &file); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, extra := range file.Providers {
		prov, ok := c.Providers[key]
		if !ok {
			continue
		}
		for _, model := range extra.Models {
			if model != "" && !contains(prov.Models, model) {
				prov.Models = append(prov.Models, model)
			}
		}
		c.Providers[key] = prov
	}
	return nil
}

func saveExtraModel(path, providerKey, model string) error {
	path, err := extraModelsPath(path)
	if err != nil {
		return err
	}
	var file extraModelsFile
	if data, err := os.ReadFile(path); err == nil {
		_ = toml.Unmarshal(data, &file)
	}
	if file.Providers == nil {
		file.Providers = map[string]extraProvider{}
	}
	cur := file.Providers[providerKey]
	if !contains(cur.Models, model) {
		cur.Models = append(cur.Models, model)
	}
	file.Providers[providerKey] = cur
	data, err := toml.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
