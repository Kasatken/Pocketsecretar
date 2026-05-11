package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pocketsecret/config"
)

// newOpenRouterHTTPClient returns a client for OpenRouter; if proxyURL is non-empty (http/https/socks5),
// all requests use that proxy; otherwise the default transport (incl. HTTP(S)_PROXY from environment).
func newOpenRouterHTTPClient(proxyURL string) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(proxyURL); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			log.Printf("openrouter: invalid OPENROUTER_PROXY, using default transport: %v", err)
		} else {
			t.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{
		Transport: t,
		Timeout:   90 * time.Second,
	}
}

// OpenRouterChat calls OpenRouter chat completions.
func OpenRouterChat(ctx context.Context, cfg config.Config, apiKey, model string, maxTok int, temperature float64, system, user string) (string, error) {
	body := map[string]interface{}{
		"model":       model,
		"temperature": temperature,
		"max_tokens":  maxTok,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	ref := strings.TrimSpace(cfg.OpenRouterReferer)
	if ref == "" {
		ref = "http://localhost:5173"
	}
	req.Header.Set("HTTP-Referer", ref)
	req.Header.Set("X-Title", "PocketSecret")

	client := newOpenRouterHTTPClient(cfg.OpenRouterProxy)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(b))
		if len(msg) > 220 {
			msg = msg[:220]
		}
		return "", fmt.Errorf("openrouter %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("пустой ответ модели")
	}
	return out.Choices[0].Message.Content, nil
}

func openRouterErrIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "connection reset") || strings.Contains(s, "broken pipe") {
		return true
	}
	if strings.Contains(s, "tls handshake") || strings.Contains(s, "i/o timeout") || strings.Contains(s, "connection refused") {
		return true
	}
	for _, code := range []string{"openrouter 429", "openrouter 502", "openrouter 503", "openrouter 504"} {
		if strings.Contains(s, code) {
			return true
		}
	}
	return false
}

// OpenRouterChatWithRetry вызывает OpenRouter до maxAttempts раз при сетевых/временных сбоях и 429/502/503 у провайдера.
func OpenRouterChatWithRetry(ctx context.Context, cfg config.Config, apiKey, model string, maxTok int, temperature float64, system, user string, maxAttempts int) (string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(400+200*attempt) * time.Millisecond
			t := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				t.Stop()
				return "", ctx.Err()
			case <-t.C:
				t.Stop()
			}
		}
		out, err := OpenRouterChat(ctx, cfg, apiKey, model, maxTok, temperature, system, user)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !openRouterErrIsRetryable(err) || attempt == maxAttempts-1 {
			return "", err
		}
		log.Printf("openrouter retry %d/%d: %v", attempt+1, maxAttempts, err)
	}
	return "", lastErr
}
