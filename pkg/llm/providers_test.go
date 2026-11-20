package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProviderGroupsGroupsAddedAPIs(t *testing.T) {
	cfg := &Config{Providers: map[string]ProviderConfig{
		"openai": {Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4o"}},
	}}
	first, err := ParseConnection("https://api.anthropic.com/v1", "sk-a", "claude-a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseConnection("https://api.anthropic.com/v1", "sk-a", "claude-b", "", "")
	if err != nil {
		t.Fatal(err)
	}
	first.Name = "Anthropic"
	second.Name = "Anthropic"
	cfg.ImportConnections([]Connection{first, second}, first.ID)

	groups := cfg.ProviderGroups()
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 user API", len(groups))
	}
	if groups[0].ID != "url:https://api.anthropic.com/v1" {
		t.Fatalf("provider = %s", groups[0].ID)
	}
	if groups[0].Label != "Anthropic · https://api.anthropic.com/v1" {
		t.Fatalf("label %q", groups[0].Label)
	}
	if len(groups[0].Models) != 2 || groups[0].Models[0] != "claude-a" || groups[0].Models[1] != "claude-b" {
		t.Fatalf("models = %#v", groups[0].Models)
	}
}

func TestAddSavedModelKeepsTheSameKey(t *testing.T) {
	cfg := &Config{}
	src, err := ParseConnection("https://api.deepseek.com/v1", "sk-deep", "deepseek-chat", "", "")
	if err != nil {
		t.Fatal(err)
	}
	src.Name = "DeepSeek"
	src.HasPrice = true
	src.PromptPrice = 0.1
	src.CompletionPrice = 0.2
	cfg.ImportConnections([]Connection{src}, src.ID)
	added, err := cfg.AddSavedModel("https://api.deepseek.com/v1", "deepseek-reasoner", "0.3", "")
	if err != nil {
		t.Fatal(err)
	}
	if added.APIKey != "sk-deep" || added.Model != "deepseek-reasoner" || added.Name != "DeepSeek" {
		t.Fatalf("added = %+v", added)
	}
	if !added.HasPrice || added.PromptPrice != 0.3 || added.CompletionPrice != 0 {
		t.Fatalf("prices %+v", added)
	}
	blank, err := cfg.AddSavedModel("https://api.deepseek.com/v1", "deepseek-chat-2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if blank.HasPrice {
		t.Fatal("blank prices should stay hidden")
	}
	if added.ID == src.ID {
		t.Fatal("a new model must get its own connection id")
	}
}

func TestAddConfigModelPersists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "extra_models.toml")
	cfg := &Config{Providers: map[string]ProviderConfig{
		"openrouter": {Name: "OpenRouter", Type: "openai", BaseURL: "https://openrouter.ai/api/v1", Models: []string{"openai/gpt-4o"}},
	}}
	if err := cfg.AddConfigModel("cfg:openrouter", "custom/model", path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	fresh := &Config{Providers: map[string]ProviderConfig{
		"openrouter": {Name: "OpenRouter", Type: "openai", BaseURL: "https://openrouter.ai/api/v1", Models: []string{"openai/gpt-4o"}},
	}}
	if err := LoadExtraModels(fresh, path); err != nil {
		t.Fatal(err)
	}
	models := fresh.Providers["openrouter"].Models
	if len(models) != 2 || models[1] != "custom/model" {
		t.Fatalf("models = %#v", models)
	}
}
