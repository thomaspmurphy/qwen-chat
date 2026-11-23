# qwen-chat

A terminal chat client for a local [ollama](https://ollama.com) model that renders answers as Markdown, with LaTeX maths turned into Unicode.

![demo](demo.gif)

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
qwen-chat --model llama3.2 --no-think 'one-line haiku about tea'
```

In the interactive chat:

- `/bye`, `/exit`, `/quit` or ctrl-d quit
- `/clear` forgets the conversation
- `/think` prints the last reply's reasoning
- `"""` opens and closes a multi-line message
- ctrl-c during a reply cancels it

While the model works, a status line on stderr shows `thinking Ns`, then `writing`. If ollama is not running and the host is local, qwen-chat starts `ollama serve` in the background (log in `~/.local/state/ollama/serve.log`).

When stdout is not a terminal, the answer is printed as plain Markdown.

## Flags and environment

| Flag | Environment | Default |
| --- | --- | --- |
| `--host` | `OLLAMA_HOST` | `http://127.0.0.1:11434` |
| `--model` | `QWEN_MODEL` | `qwen3.5:4b` |
| `--no-think` | | reasoning on |
| `--version` | | |

## Licence

MIT
