package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type message struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	Thinking string `json:"thinking,omitempty"`
}

type client struct {
	host      string
	keepAlive string
	settings
}

func normaliseHost(h string) string {
	h = strings.TrimRight(h, "/")
	if !strings.HasPrefix(h, "http") {
		h = "http://" + h
	}
	return h
}

func (c *client) up(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", c.host+"/api/version", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *client) local() bool {
	u, err := url.Parse(c.host)
	if err != nil {
		return false
	}
	h := u.Hostname()
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// Only a local host is started: a remote one is someone else's server.
func (c *client) ensureServer(ctx context.Context) error {
	if c.up(ctx) {
		return nil
	}
	bin, err := exec.LookPath("ollama")
	if err != nil || !c.local() {
		return fmt.Errorf("no ollama server at %s (install ollama from https://ollama.com, or run `ollama serve`)", c.host)
	}
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".local", "state", "ollama")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "serve.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(bin, "serve")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = serveEnv(os.Environ(), c.host)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ollama serve: %w", err)
	}
	cmd.Process.Release()
	dim(os.Stderr, "started ollama serve (log: "+logPath+")")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c.up(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("ollama serve did not answer at %s within 10s; see %s", c.host, logPath)
}

func serveEnv(env []string, host string) []string {
	env = append([]string(nil), env...)
	if u, err := url.Parse(host); err == nil {
		env = append(env, "OLLAMA_HOST="+u.Host)
	}
	// What Homebrew's ollama service sets: they cut the memory a long context needs.
	for _, kv := range []string{"OLLAMA_FLASH_ATTENTION=1", "OLLAMA_KV_CACHE_TYPE=q8_0"} {
		k, _, _ := strings.Cut(kv, "=")
		if _, set := os.LookupEnv(k); !set {
			env = append(env, kv)
		}
	}
	return env
}

func (c *client) requestBody(messages []message) ([]byte, error) {
	return json.Marshal(map[string]any{
		"model":    c.model,
		"messages": messages,
		"stream":   true,
		"think":    c.think,
		// Ollama unloads a model after 5 minutes by default, so each one-shot
		// a little apart would pay the load time again.
		"keep_alive": c.keepAlive,
		"options":    map[string]any{"num_ctx": c.numCtx},
	})
}

type chunk struct {
	Message message `json:"message"`
	Error   string  `json:"error"`
}

func (c *client) ask(ctx context.Context, messages []message, drawStatus bool) (string, string, error) {
	body, err := c.requestBody(messages)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	st := newStatus(drawStatus, c.think)
	defer st.close()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		var ch chunk
		if json.Unmarshal(b, &ch) == nil && ch.Error != "" {
			if resp.StatusCode == http.StatusNotFound && strings.Contains(ch.Error, "not found") {
				return "", "", &missingModelError{model: c.model, mode: c.mode}
			}
			return "", "", errors.New(ch.Error)
		}
		return "", "", fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}

	var answer strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ch chunk
		if err := json.Unmarshal(line, &ch); err != nil {
			return "", "", fmt.Errorf("bad response line: %w", err)
		}
		if ch.Error != "" {
			return "", "", errors.New(ch.Error)
		}
		if ch.Message.Thinking != "" {
			st.addThinking(ch.Message.Thinking)
		}
		if ch.Message.Content != "" {
			st.writing()
			answer.WriteString(ch.Message.Content)
		}
	}
	if err := sc.Err(); err != nil {
		return "", "", err
	}
	st.close()
	thinking := string(st.thinking)
	if st.thoughtFor > 0 && strings.TrimSpace(thinking) != "" {
		dim(os.Stdout, fmt.Sprintf("✻ thought for %ds", int(st.thoughtFor.Seconds())))
	}
	return answer.String(), thinking, nil
}
