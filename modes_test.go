package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseFlags(t *testing.T) {
	parse := func(args ...string) (options, error) {
		fs := flag.NewFlagSet("qwen-chat", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		return parseFlags(fs, args)
	}
	if o, err := parse(); err != nil || o.think || o.deep {
		t.Errorf("no flags: %+v, %v", o, err)
	}
	if o, err := parse("-t", "-d"); err != nil || !o.think || !o.deep {
		t.Errorf("-t -d: %+v, %v", o, err)
	}
	if o, err := parse("--think", "--deep"); err != nil || !o.think || !o.deep {
		t.Errorf("--think --deep: %+v, %v", o, err)
	}
	for _, gone := range []string{"-q", "--quick", "--no-think"} {
		if _, err := parse(gone); err == nil {
			t.Errorf("%s should no longer be accepted", gone)
		}
	}
}

func TestResolve(t *testing.T) {
	none := envOf(nil)
	for _, tc := range []struct {
		name  string
		mode  string
		think bool
		env   func(string) string
		want  settings
	}{
		{"default", modeDefault, false, none, settings{modeDefault, "qwen3.5:4b", false, 8192}},
		{"deep thinks by default", modeDeep, false, none, settings{modeDeep, "qwen3.5:9b", true, 16384}},
		{"default think", modeDefault, true, none, settings{modeDefault, "qwen3.5:4b", true, 8192}},
		{"deep think", modeDeep, true, none, settings{modeDeep, "qwen3.5:9b", true, 16384}},
		{"default env", modeDefault, false, envOf(map[string]string{"QWEN_MODEL": "a:1b"}), settings{modeDefault, "a:1b", false, 8192}},
		{"deep env", modeDeep, false, envOf(map[string]string{"QWEN_DEEP_MODEL": "d:1b", "QWEN_MODEL": "a:1b"}), settings{modeDeep, "d:1b", true, 16384}},
	} {
		if got := resolve(tc.mode, tc.think, tc.env); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestModelFlagOnlyForStartingMode(t *testing.T) {
	env := envOf(map[string]string{"QWEN_MODEL": "a:1b"})
	start := startSettings(modeDefault, "x", false, env)
	if start.model != "x" || start.mode != modeDefault {
		t.Errorf("start: %+v, want model x in default mode", start)
	}
	if deep := resolve(modeDeep, false, env); deep.model != "qwen3.5:9b" {
		t.Errorf("after /deep: model %s, want qwen3.5:9b", deep.model)
	}
	if back := resolve(modeDefault, false, env); back.model != "a:1b" {
		t.Errorf("after /default: model %s, want a:1b", back.model)
	}
	if s := startSettings(modeDeep, "", true, env); s.model != "qwen3.5:9b" || !s.think {
		t.Errorf("deep start without --model: %+v", s)
	}
}

func TestForPiped(t *testing.T) {
	s := resolve(modeDefault, false, envOf(nil))
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
	c := &client{keepAlive: "30m", settings: resolve(modeDefault, false, envOf(nil)).forPiped(true)}
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
	c.settings = resolve(modeDefault, true, envOf(nil))
	if b, _ := c.requestBody(nil); !strings.Contains(string(b), `"think":true`) {
		t.Errorf("with --think, request body %s", b)
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
	c := &client{host: srv.URL, keepAlive: "30m", settings: resolve(modeDeep, false, envOf(map[string]string{"QWEN_DEEP_MODEL": "big:35b"}))}
	_, _, err := c.ask(context.Background(), []message{{Role: "user", Content: "hi"}}, false)
	var missing *missingModelError
	if !errors.As(err, &missing) || missing.model != "big:35b" || missing.mode != modeDeep {
		t.Errorf("got %v, want a missing-model error for big:35b in deep mode", err)
	}
}
