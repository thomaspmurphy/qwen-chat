package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

//go:embed styles/autumn-night.json
var styleJSON []byte

func render(w io.Writer, md string, tty bool, width int) error {
	md = renderTeX(md)
	if !tty {
		_, err := fmt.Fprintln(w, md)
		return err
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(styleJSON),
		glamour.WithWordWrap(width),
		// terminal256 (glow's choice) rounds the style's hex colours; the
		// palette is only exact in truecolour.
		glamour.WithChromaFormatter("terminal16m"),
	)
	if err != nil {
		return err
	}
	out, err := r.Render(md)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, out)
	return err
}

func dim(w io.Writer, s string) {
	s = strings.TrimRight(s, "\n")
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		s = "\033[2m" + s + "\033[0m"
	}
	fmt.Fprintln(w, s)
}
