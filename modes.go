package main

import (
	"errors"
	"fmt"
)

const (
	modeQuick   = "quick"
	modeDefault = "default"
	modeDeep    = "deep"
)

type modeSpec struct {
	modelEnv string
	model    string
	think    bool
	numCtx   int
}

// The defaults for each mode. Change a model, thinking or context size here.
var modeSpecs = map[string]modeSpec{
	modeQuick:   {modelEnv: "QWEN_QUICK_MODEL", model: "qwen3.5:4b", think: false, numCtx: 8192},
	modeDefault: {modelEnv: "QWEN_MODEL", model: "qwen3.5:4b", think: true, numCtx: 8192},
	modeDeep:    {modelEnv: "QWEN_DEEP_MODEL", model: "qwen3.6:35b-a3b", think: true, numCtx: 16384},
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

func pickMode(quick, deep bool) (string, error) {
	switch {
	case quick && deep:
		return "", errors.New("-q/--quick and -d/--deep cannot be used together")
	case quick:
		return modeQuick, nil
	case deep:
		return modeDeep, nil
	}
	return modeDefault, nil
}

// modelFlag is the --model value, empty when not given.
func resolve(mode, modelFlag string, noThink bool, getenv func(string) string) settings {
	spec := modeSpecs[mode]
	s := settings{mode: mode, model: spec.model, think: spec.think && !noThink, numCtx: spec.numCtx}
	if v := getenv(spec.modelEnv); v != "" {
		s.model = v
	}
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
