package main

import "fmt"

const (
	modeDefault = "default"
	modeDeep    = "deep"
)

type modeSpec struct {
	modelEnv string
	model    string
	numCtx   int
	think    bool
}

// The defaults for each mode. Change a model or context size here.
// Measured on a 24 GB M5 Pro: without thinking the 9b was no more accurate
// than the 4b and sometimes wrong where the 4b was right; with thinking it
// was right and faster than the 4b thinking. Deep is opt-in, so it thinks.
// qwen3.6:35b-a3b needs 21.5 GB of GPU memory, more than macOS gives the GPU
// on a 24 GB Mac.
var modeSpecs = map[string]modeSpec{
	modeDefault: {modelEnv: "QWEN_MODEL", model: "qwen3.5:4b", numCtx: 8192},
	modeDeep:    {modelEnv: "QWEN_DEEP_MODEL", model: "qwen3.5:9b", numCtx: 16384, think: true},
}

const (
	defaultKeepAlive = "30m"
	pipedNumCtx      = 16384
)

type settings struct {
	mode   string
	model  string
	think  bool
	numCtx int
}

func resolve(mode string, think bool, getenv func(string) string) settings {
	spec := modeSpecs[mode]
	s := settings{mode: mode, model: spec.model, think: think || spec.think, numCtx: spec.numCtx}
	if v := getenv(spec.modelEnv); v != "" {
		s.model = v
	}
	return s
}

// --model names the model for the starting mode only, so switching mode in
// the chat actually changes the model.
func startSettings(mode, modelFlag string, think bool, getenv func(string) string) settings {
	s := resolve(mode, think, getenv)
	if modelFlag != "" {
		s.model = modelFlag
	}
	return s
}

// Ollama truncates the prompt silently past num_ctx, so a piped file would
// lose its start without this.
func (s settings) forPiped(piped bool) settings {
	if piped && s.numCtx < pipedNumCtx {
		s.numCtx = pipedNumCtx
	}
	return s
}

type missingModelError struct {
	model string
	mode  string
}

func (e *missingModelError) Error() string {
	msg := fmt.Sprintf("model %s is not installed. Run: ollama pull %s", e.model, e.model)
	if e.mode == modeDeep {
		msg += " (or set QWEN_DEEP_MODEL to a model you have)"
	}
	return msg
}
