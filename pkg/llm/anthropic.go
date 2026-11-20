package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func init() {
	RegisterProviderFactory("anthropic", func(apiKey, baseURL string) Provider {
		return NewAnthropicProvider(apiKey, baseURL)
	})
}

// AnthropicProvider calls Anthropic's Messages API.
type AnthropicProvider struct {
	APIKey  string
	BaseURL string
	Client  *http.Client
}

func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{APIKey: apiKey, BaseURL: baseURL, Client: &http.Client{}}
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Generate(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request cannot be nil")
	}
	payload, err := buildAnthropicPayload(req)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL(p.BaseURL), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if p.APIKey != "" {
		httpReq.Header.Set("x-api-key", p.APIKey)
	}
	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request to LLM failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read LLM response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, raw)
	}
	var parsed anthropicResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode LLM response: %w", err)
	}
	out := &ChatResponse{Usage: Usage{
		PromptTokens:     parsed.Usage.InputTokens,
		CompletionTokens: parsed.Usage.OutputTokens,
		TotalTokens:      parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
	}}
	var text strings.Builder
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			if text.Len() > 0 {
				text.WriteByte('\n')
			}
			text.WriteString(block.Text)
		case "tool_use":
			args := string(block.Input)
			if args == "" || args == "null" {
				args = "{}"
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}
	out.Content = text.String()
	return out, nil
}

func anthropicMessagesURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/messages"
	}
	return base + "/v1/messages"
}

type anthropicPayload struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

type anthropicResponse struct {
	Content []anthropicBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicToolResult struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

type anthropicBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

func buildAnthropicPayload(req *ChatRequest) (anthropicPayload, error) {
	msgs := NormalizeMessages(req.Message)
	payload := anthropicPayload{Model: req.Model, MaxTokens: 8192}
	var system strings.Builder
	var pending []anthropicToolResult
	flushTools := func() {
		if len(pending) == 0 {
			return
		}
		payload.Messages = append(payload.Messages, anthropicMessage{Role: "user", Content: pending})
		pending = nil
	}
	for _, msg := range msgs {
		switch msg.Role {
		case RoleSystem:
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(msg.Content)
		case RoleUser:
			flushTools()
			payload.Messages = append(payload.Messages, anthropicMessage{Role: "user", Content: msg.Content})
		case RoleAssistant:
			flushTools()
			var blocks []anthropicBlock
			if strings.TrimSpace(msg.Content) != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: msg.Content})
			}
			for _, call := range msg.ToolCalls {
				raw := json.RawMessage(call.Function.Arguments)
				if !json.Valid(raw) {
					encoded, _ := json.Marshal(call.Function.Arguments)
					raw = encoded
				}
				blocks = append(blocks, anthropicBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: raw})
			}
			if len(blocks) == 0 {
				blocks = []anthropicBlock{{Type: "text", Text: msg.Content}}
			}
			payload.Messages = append(payload.Messages, anthropicMessage{Role: "assistant", Content: blocks})
		case RoleTool:
			pending = append(pending, anthropicToolResult{Type: "tool_result", ToolUseID: msg.ToolCallID, Content: msg.Content})
		}
	}
	flushTools()
	payload.System = system.String()
	for _, tool := range req.Tools {
		schema := tool.Function.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		payload.Tools = append(payload.Tools, anthropicTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: schema,
		})
	}
	if len(payload.Messages) == 0 {
		return payload, fmt.Errorf("anthropic request has no messages")
	}
	return payload, nil
}
