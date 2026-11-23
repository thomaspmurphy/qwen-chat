package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/ergochat/readline"
	"golang.org/x/term"
)

var version = "dev"

const usage = `qwen-chat: chat with a local ollama model, answers rendered as Markdown.

  qwen-chat                      interactive chat
  qwen-chat how do I x           one-shot
  cat f.ex | qwen-chat explain   one-shot with the piped text added to the prompt

Modes: -q quick (no reasoning), default, -d deep (bigger model, longer
context).

Interactive commands: /bye /exit /quit or ctrl-d quit, /clear forgets the
conversation, /think prints the last reasoning, /quick /default /deep switch
mode, """ opens and closes a multi-line message.

Flags:
`

func main() {
	os.Exit(run())
}

func run() int {
	env := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	host := flag.String("host", env("OLLAMA_HOST", "http://127.0.0.1:11434"), "ollama server (env OLLAMA_HOST)")
	model := flag.String("model", "", "model name, overriding the mode's (env QWEN_MODEL, QWEN_QUICK_MODEL, QWEN_DEEP_MODEL)")
	noThink := flag.Bool("no-think", false, "ask the model not to reason before answering")
	var quick, deep bool
	flag.BoolVar(&quick, "q", false, "quick mode: no reasoning")
	flag.BoolVar(&quick, "quick", false, "quick mode: no reasoning")
	flag.BoolVar(&deep, "d", false, "deep mode: bigger model, longer context")
	flag.BoolVar(&deep, "deep", false, "deep mode: bigger model, longer context")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("qwen-chat", version)
		return 0
	}

	mode, err := pickMode(quick, deep)
	if err != nil {
		fmt.Fprintln(os.Stderr, "qwen-chat:", err)
		return 2
	}
	pick := func(mode string) settings { return resolve(mode, *model, *noThink, os.Getenv) }
	c := &client{host: normaliseHost(*host), keepAlive: env("QWEN_KEEP_ALIVE", defaultKeepAlive), settings: pick(mode)}
	stdinTTY := term.IsTerminal(int(os.Stdin.Fd()))
	prompt := strings.Join(flag.Args(), " ")
	piped := ""
	if !stdinTTY {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "qwen-chat:", err)
			return 1
		}
		piped = string(b)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err = c.ensureServer(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "qwen-chat:", err)
		return 1
	}

	if prompt == "" && piped == "" {
		if !stdinTTY {
			flag.Usage()
			return 2
		}
		return interactive(c, pick)
	}
	content := prompt
	if piped != "" {
		content = strings.TrimSpace(prompt + "\n\n" + piped)
	}
	c.settings = c.settings.forPiped(strings.TrimSpace(piped) != "")
	ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	answer, _, err := c.ask(ctx, []message{{Role: "user", Content: content}}, stderrTTY())
	if ctx.Err() != nil {
		fmt.Println()
		return 130
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "qwen-chat:", err)
		return 1
	}
	show(answer)
	return 0
}

func stderrTTY() bool { return term.IsTerminal(int(os.Stderr.Fd())) }

func show(answer string) {
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}
	if err := render(os.Stdout, answer, tty, width); err != nil {
		fmt.Println(answer)
	}
}

// """ opens and closes a multi-line message, as in ollama's own prompt.
func readPrompt(rl *readline.Instance, mode string) (string, error) {
	p := "\033[38;2;255;193;7m›\033[0m "
	if mode != modeDefault {
		p = "\033[2m" + mode + "\033[0m " + p
	}
	rl.SetPrompt(p)
	line, err := rl.ReadLine()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(line), `"""`) {
		return line, err
	}
	lines := []string{strings.TrimSpace(line)[3:]}
	rl.SetPrompt("  ")
	for {
		next, err := rl.ReadLine()
		if err != nil {
			return "", err
		}
		if t := strings.TrimRight(next, " \t"); strings.HasSuffix(t, `"""`) {
			lines = append(lines, t[:len(t)-3])
			return strings.TrimSpace(strings.Join(lines, "\n")), nil
		}
		lines = append(lines, next)
	}
}

func interactive(c *client, pick func(mode string) settings) int {
	rl, err := readline.NewFromConfig(&readline.Config{InterruptPrompt: "\n", EOFPrompt: "\n"})
	if err != nil {
		fmt.Fprintln(os.Stderr, "qwen-chat:", err)
		return 1
	}
	defer rl.Close()
	dim(os.Stdout, c.mode+" · "+c.model+` · /bye or ctrl-d to quit, /clear to forget, /think for the last reasoning, /quick /default /deep to switch mode, """ for multi-line`)

	var messages []message
	lastThinking := ""
	for {
		prompt, err := readPrompt(rl, c.mode)
		if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
			return 0
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "qwen-chat:", err)
			return 1
		}
		switch strings.TrimSpace(prompt) {
		case "":
			continue
		case "/bye", "/exit", "/quit":
			return 0
		case "/think":
			if t := strings.TrimSpace(lastThinking); t != "" {
				dim(os.Stdout, t)
			} else {
				dim(os.Stdout, "no reasoning yet")
			}
			continue
		case "/quick", "/default", "/deep":
			c.settings = pick(strings.TrimSpace(prompt)[1:])
			dim(os.Stdout, c.mode+" mode · "+c.model)
			continue
		case "/clear":
			messages = nil
			dim(os.Stdout, "forgotten")
			continue
		}

		messages = append(messages, message{Role: "user", Content: prompt})
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		answer, thinking, err := c.ask(ctx, messages, stderrTTY())
		cancelled := ctx.Err() != nil
		stop()
		if cancelled || err != nil {
			messages = messages[:len(messages)-1]
			if cancelled {
				fmt.Println()
			} else {
				fmt.Fprintln(os.Stderr, "qwen-chat:", err)
			}
			continue
		}
		lastThinking = thinking
		messages = append(messages, message{Role: "assistant", Content: answer})
		show(answer)
	}
}
