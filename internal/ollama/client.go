// Package ollama is a thin HTTP client for a local Ollama server: text
// embeddings for semantic matching and chat completions for verification.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pgvector/pgvector-go"
)

type Client struct {
	baseURL    string
	embedModel string
	chatModel  string
	http       *http.Client
}

func New(baseURL, embedModel, chatModel string) *Client {
	return &Client{
		baseURL:    baseURL,
		embedModel: embedModel,
		chatModel:  chatModel,
		http:       &http.Client{Timeout: 60 * time.Second},
	}
}

// Embedder is the subset reports/matching depend on, so it can be mocked in tests.
type Embedder interface {
	Embed(ctx context.Context, text string) (pgvector.Vector, error)
}

// Chatter is the subset verification depends on.
type Chatter interface {
	Chat(ctx context.Context, system, user string, jsonMode bool) (string, error)
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns the embedding of text as a pgvector.Vector ready to store.
func (c *Client) Embed(ctx context.Context, text string) (pgvector.Vector, error) {
	var out embedResponse
	if err := c.post(ctx, "/api/embeddings", embedRequest{Model: c.embedModel, Prompt: text}, &out); err != nil {
		return pgvector.Vector{}, err
	}
	if len(out.Embedding) == 0 {
		return pgvector.Vector{}, fmt.Errorf("ollama: empty embedding (is model %q pulled?)", c.embedModel)
	}
	return pgvector.NewVector(out.Embedding), nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

// Chat runs a single-turn completion. When jsonMode is true, Ollama is asked to
// constrain output to valid JSON.
func (c *Client) Chat(ctx context.Context, system, user string, jsonMode bool) (string, error) {
	req := chatRequest{
		Model:    c.chatModel,
		Stream:   false,
		Messages: []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}},
	}
	if jsonMode {
		req.Format = "json"
	}
	var out chatResponse
	if err := c.post(ctx, "/api/chat", req, &out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("ollama: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return fmt.Errorf("ollama: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("ollama: decode: %w", err)
	}
	return nil
}
