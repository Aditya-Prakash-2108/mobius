package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestConnectionOmitsPriceWhenBlank(t *testing.T) {
	conn, err := ParseConnection("https://example.com/v1/", "sk-test", "my-model", "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if conn.HasPrice {
		t.Fatal("blank prices should not count as a price")
	}
	if conn.BaseURL != "https://example.com/v1" {
		t.Fatalf("base url %q", conn.BaseURL)
	}

	cfg := &Config{}
	if err := cfg.UseConnection(conn); err != nil {
		t.Fatal(err)
	}
	if cfg.PricesKnown("my-model") {
		t.Fatal("price should stay unknown")
	}
	pCost, cCost := cfg.GetPrices("my-model")
	if pCost != 0 || cCost != 0 {
		t.Fatalf("prices = %v %v", pCost, cCost)
	}
	provider, err := cfg.GetProviderForModel("")
	if err != nil {
		t.Fatal(err)
	}
	got := provider.(*OpenAIProvider)
	if got.APIKey != "sk-test" || got.BaseURL != "https://example.com/v1" {
		t.Fatalf("provider key %q url %q", got.APIKey, got.BaseURL)
	}

	path := filepath.Join(t.TempDir(), "connection.toml")
	if err := SaveConnection(path, conn); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "price") || strings.Contains(string(data), "api_key") {
		t.Fatalf("secret or price written into %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %v", info.Mode().Perm())
	}
	loaded, err := LoadConnection(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HasPrice || loaded.APIKey != "sk-test" || loaded.Model != "my-model" {
		t.Fatalf("loaded %+v", loaded)
	}
}

func TestConnectionKeepsEnteredPrice(t *testing.T) {
	conn, err := ParseConnection("https://example.com/v1", "sk-test", "my-model", "0.10", "0.20")
	if err != nil {
		t.Fatal(err)
	}
	if !conn.HasPrice || conn.PromptPrice != 0.10 || conn.CompletionPrice != 0.20 {
		t.Fatalf("%+v", conn)
	}
	cfg := &Config{}
	if err := cfg.UseConnection(conn); err != nil {
		t.Fatal(err)
	}
	if !cfg.PricesKnown("my-model") {
		t.Fatal("entered price should be known")
	}
	form := cfg.ConnectionForm()
	if form.PromptPrice != "0.1" || !form.HasKey {
		t.Fatalf("form %+v", form)
	}
}

func TestSavedAPIsStaySeparate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "connections.toml")
	first, err := ParseConnection("https://api.anthropic.com", "sk-ant", "claude-3-5-sonnet", "", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseConnection("https://openrouter.ai/api/v1", "sk-or", "deepseek/deepseek-v4.1-flash", "0.1", "0.2")
	if err != nil {
		t.Fatal(err)
	}
	if first.Protocol != "anthropic" || second.Protocol != "openai" {
		t.Fatalf("protocols %s %s", first.Protocol, second.Protocol)
	}
	if err := SaveConnection(path, first); err != nil {
		t.Fatal(err)
	}
	if err := SaveConnection(path, second); err != nil {
		t.Fatal(err)
	}
	conns, active, err := LoadSaved(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 2 {
		t.Fatalf("got %d connections", len(conns))
	}
	if active != second.ID {
		t.Fatalf("active %s", active)
	}
	if conns[0].APIKey != "sk-ant" || conns[1].APIKey != "sk-or" {
		t.Fatalf("keys did not round-trip from the keystore")
	}
	cfg := &Config{}
	cfg.ImportConnections(conns, active)
	provider, err := cfg.GetProviderForModel("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*AnthropicProvider); ok {
		t.Fatal("active provider should be the OpenRouter endpoint")
	}
	anth, err := cfg.GetProviderForModel("claude-3-5-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := anth.(*AnthropicProvider); !ok {
		t.Fatalf("anthropic URL used %T", anth)
	}
	legacy := filepath.Join(dir, "connection.toml")
	if err := os.WriteFile(legacy, []byte("base_url = \"https://api.deepseek.com/v1\"\nmodel = \"deepseek-chat\"\napi_key = \"sk-old\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "fresh.toml")
	if _, _, err := LoadSaved(moved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("plaintext key file should be removed after migration")
	}
	data, err := os.ReadFile(moved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-old") || strings.Contains(string(data), "api_key") {
		t.Fatalf("migrated file still has the key: %s", data)
	}
}

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
