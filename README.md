# qwen-chat

A terminal chat client for a local [ollama](https://ollama.com) model that renders answers as Markdown, with LaTeX maths turned into Unicode.

![qwen-chat drawing the quadratic formula, writing a highlighted Python function, then explaining a piped file](demo.gif)

## Dependencies

- **ollama**, required: <https://ollama.com>, or `brew install ollama` on macOS.
- **A model**, required: `ollama pull qwen3.5:4b` (about 4 GB). Any other model works with `--model` or `QWEN_MODEL`.
- **utftex**, optional, for LaTeX maths: `brew install utftex`. Without it LaTeX is shown as written.
- **A truecolour terminal**, for the exact colours.
- **Go 1.27+**, only if you install with `go install`.

It is a single static binary: no Python, glow or other runtime is needed.

## Install

Download the archive for your platform from the [releases page](https://github.com/thomaspmurphy/qwen-chat/releases) and put the binary on your `PATH`:

```sh
tar -xzf qwen-chat_*_darwin_arm64.tar.gz qwen-chat
mkdir -p ~/.local/bin && mv qwen-chat ~/.local/bin/
```

On macOS, a downloaded binary is quarantined; clear the flag once:

```sh
xattr -d com.apple.quarantine ~/.local/bin/qwen-chat
```

Or build it with Go:

```sh
go install github.com/thomaspmurphy/qwen-chat@latest
```

## Usage

```sh
qwen-chat                              # interactive chat
qwen-chat how do I reverse a list in elixir
cat lib/foo.ex | qwen-chat explain this
qwen-chat -d prove sqrt 2 is irrational  # deep: bigger model
qwen-chat --think 'is 1009 prime?'       # let the model reason first
qwen-chat --model llama3.2 'one-line haiku about tea'
```

In the interactive chat:

- `/bye`, `/exit`, `/quit` or ctrl-d quit
- `/clear` forgets the conversation
- `/think` prints the last reply's reasoning (with `--think`)
- `/default`, `/deep` switch mode for the next messages, keeping the conversation
- `"""` opens and closes a multi-line message
- ctrl-c during a reply cancels it

While the model works, a status line on stderr shows `waiting Ns` (or `thinking Ns` with `--think`), then `writing`. If ollama is not running and the host is local, qwen-chat starts `ollama serve` in the background (log in `~/.local/state/ollama/serve.log`), with flash attention and a q8_0 KV cache unless `OLLAMA_FLASH_ATTENTION` or `OLLAMA_KV_CACHE_TYPE` are set.

When stdout is not a terminal, the answer is printed as plain Markdown.

## Modes

| Mode | Flag | Chat command | Default model | Download | Thinking | Context |
| --- | --- | --- | --- | --- | --- | --- |
| default | | `/default` | `qwen3.5:4b` | about 4 GB | off | 8192 |
| deep | `-d`, `--deep` | `/deep` | `qwen3.5:9b` | about 8 GB | on | 16384 |

Default mode answers in a second or two. Deep mode is for questions where you'd rather wait and be right: on a 24 GB M5 Pro, a tricky bug question took the 4b without thinking 1 s and the 9b with thinking 45 s to 2 minutes across runs, and only the thinking answer was fully right. Without thinking the 9b was no better than the 4b, so deep thinks. `-t` or `--think` turns thinking on in default mode too.

On a Mac with 32 GB or more, `QWEN_DEEP_MODEL=qwen3.6:35b-a3b` is a stronger deep model (about 23 GB). On 24 GB it doesn't load: it needs 21.5 GB of GPU memory and macOS gives the GPU about 17 GB.

Only the default model needs pulling to get started. Deep is opt-in: run `ollama pull qwen3.5:9b` if you want it. Nothing is downloaded automatically; a missing model prints the `ollama pull` command to run.

`--model` replaces the starting mode's model, including when the chat switches back to that mode; the other mode uses its own model. Piped input raises the context to at least 16384; ollama sets the context when it loads a model, so this applies only if the model isn't already loaded. The model stays loaded for 30 minutes after each reply, so the next one-shot starts quickly.

## Flags and environment

| Flag | Environment | Default |
| --- | --- | --- |
| `--host` | `OLLAMA_HOST` | `http://127.0.0.1:11434` |
| `--model` | `QWEN_MODEL` | `qwen3.5:4b` (default mode) |
| | `QWEN_DEEP_MODEL` | `qwen3.5:9b` |
| | `QWEN_KEEP_ALIVE` | `30m` |
| `-d`, `--deep` | | off |
| `-t`, `--think` | | off |
| `--version` | | |

## Licence

MIT
