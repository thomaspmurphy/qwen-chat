package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestPickMode(t *testing.T) {
	for _, tc := range []struct {
		quick, deep bool
		want        string
	}{
		{false, false, modeDefault},
		{true, false, modeQuick},
		{false, true, modeDeep},
	} {
		got, err := pickMode(tc.quick, tc.deep)
		if err != nil || got != tc.want {
			t.Errorf("pickMode(%v, %v) = %q, %v; want %q", tc.quick, tc.deep, got, err, tc.want)
		}
	}
	if _, err := pickMode(true, true); err == nil {
		t.Error("pickMode(true, true) should fail")
	}
}

func TestResolve(t *testing.T) {
	none := envOf(nil)
	for _, tc := range []struct {
		name      string
		mode      string
		modelFlag string
		noThink   bool
		env       func(string) string
		want      settings
	}{
		{"default", modeDefault, "", false, none, settings{modeDefault, "qwen3.5:4b", true, 8192}},
		{"quick", modeQuick, "", false, none, settings{modeQuick, "qwen3.5:4b", false, 8192}},
		{"deep", modeDeep, "", false, none, settings{modeDeep, "qwen3.6:35b-a3b", true, 16384}},
		{"default env", modeDefault, "", false, envOf(map[string]string{"QWEN_MODEL": "a:1b"}), settings{modeDefault, "a:1b", true, 8192}},
		{"quick env", modeQuick, "", false, envOf(map[string]string{"QWEN_QUICK_MODEL": "q:1b", "QWEN_MODEL": "a:1b"}), settings{modeQuick, "q:1b", false, 8192}},
		{"deep env", modeDeep, "", false, envOf(map[string]string{"QWEN_DEEP_MODEL": "d:1b", "QWEN_MODEL": "a:1b"}), settings{modeDeep, "d:1b", true, 16384}},
		{"flag beats env", modeDeep, "m:1b", false, envOf(map[string]string{"QWEN_DEEP_MODEL": "d:1b"}), settings{modeDeep, "m:1b", true, 16384}},
		{"flag in quick", modeQuick, "m:1b", false, none, settings{modeQuick, "m:1b", false, 8192}},
		{"no-think", modeDeep, "", true, none, settings{modeDeep, "qwen3.6:35b-a3b", false, 16384}},
	} {
		if got := resolve(tc.mode, tc.modelFlag, tc.noThink, tc.env); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestForPiped(t *testing.T) {
	s := resolve(modeQuick, "", false, envOf(nil))
	if got := s.forPiped(false).numCtx; got != 8192 {
		t.Errorf("no piped input: num_ctx %d, want 8192", got)
	}
	p := s.forPiped(true)
	if p.numCtx != 16384 || p.model != s.model {
		t.Errorf("piped input: got %+v, want num_ctx 16384 and model unchanged", p)
	}
	big := settings{numCtx: 32768}
	if got := big.forPiped(true).numCtx; got != 32768 {
		t.Errorf("piped input lowered num_ctx to %d", got)
	}
}

func TestRequestBody(t *testing.T) {
	c := &client{keepAlive: "30m", settings: resolve(modeQuick, "", false, envOf(nil)).forPiped(true)}
	b, err := c.requestBody([]message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Model     string `json:"model"`
		Think     bool   `json:"think"`
		KeepAlive string `json:"keep_alive"`
		Options   struct {
			NumCtx int `json:"num_ctx"`
		} `json:"options"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Model != "qwen3.5:4b" || got.Think || got.KeepAlive != "30m" || got.Options.NumCtx != 16384 {
		t.Errorf("request body %s", b)
	}
}

func TestMissingModelError(t *testing.T) {
	plain := (&missingModelError{model: "nope:1b", mode: modeDefault}).Error()
	if plain != "model nope:1b is not installed. Run: ollama pull nope:1b" {
		t.Errorf("plain: %q", plain)
	}
	deep := (&missingModelError{model: "big:35b", mode: modeDeep}).Error()
	if deep != "model big:35b is not installed. Run: ollama pull big:35b (or set QWEN_DEEP_MODEL to a model you have)" {
		t.Errorf("deep: %q", deep)
	}
}

func TestServeEnv(t *testing.T) {
	t.Setenv("OLLAMA_KV_CACHE_TYPE", "f16")
	t.Setenv("OLLAMA_FLASH_ATTENTION", "")
	os.Unsetenv("OLLAMA_FLASH_ATTENTION")
	env := strings.Join(serveEnv([]string{"OLLAMA_KV_CACHE_TYPE=f16"}, "http://127.0.0.1:11434"), "\n")
	if !strings.Contains(env, "OLLAMA_FLASH_ATTENTION=1") || strings.Contains(env, "q8_0") || !strings.Contains(env, "OLLAMA_HOST=127.0.0.1:11434") {
		t.Errorf("serveEnv:\n%s", env)
	}
}

func TestAskMissingModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"model 'big:35b' not found"}`))
	}))
	defer srv.Close()
	c := &client{host: srv.URL, keepAlive: "30m", settings: resolve(modeDeep, "", false, envOf(map[string]string{"QWEN_DEEP_MODEL": "big:35b"}))}
	_, _, err := c.ask(context.Background(), []message{{Role: "user", Content: "hi"}}, false)
	var missing *missingModelError
	if !errors.As(err, &missing) || missing.model != "big:35b" || missing.mode != modeDeep {
		t.Errorf("got %v, want a missing-model error for big:35b in deep mode", err)
	}
}
