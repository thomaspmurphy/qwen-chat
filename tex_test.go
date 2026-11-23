package main

import (
	"bytes"
	"strings"
	"testing"
)

func needUtftex(t *testing.T) {
	t.Helper()
	if utftexPath == "" {
		t.Skip("utftex not on PATH")
	}
}

func TestDisplayBlock(t *testing.T) {
	needUtftex(t)
	got := renderTeX("Before\n$$\\frac{a}{b}$$\nAfter\n")
	if !strings.Contains(got, "```text\n") || strings.Contains(got, "$$") || strings.Contains(got, `\frac`) {
		t.Fatalf("display maths not drawn as a text block:\n%s", got)
	}
	if !strings.HasPrefix(got, "Before\n") || !strings.HasSuffix(got, "After\n") {
		t.Fatalf("surrounding text changed:\n%s", got)
	}
}

func TestInlineSimple(t *testing.T) {
	needUtftex(t)
	for in, want := range map[string]string{
		`area $\pi r^2$ here`: "area πr² here",
		`so \(\alpha\) is`:    "so α is",
	} {
		if got := renderTeX(in); got != want {
			t.Errorf("renderTeX(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInlineFlatFallback(t *testing.T) {
	needUtftex(t)
	for in, want := range map[string]string{
		`$\int_0^\infty e^{-x^2}dx$`:             "∫₀^∞ e⁻ˣ²dx",
		`$\sum_{i=1}^{n} x_i$`:                   "∑ᵢ₌₁ⁿ xᵢ",
		`$\sqrt{x^2+1}$`:                         "√(x²+1)",
		`$x = \frac{-b \pm \sqrt{b^2-4ac}}{2a}$`: "x = (-b ± √(b²-4ac))/(2a)",
	} {
		if got := renderTeX(in); got != want {
			t.Errorf("renderTeX(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnchanged(t *testing.T) {
	for _, in := range []string{
		"It costs $5 and $10.",
		"Use `$HOME` for that.",
		"echo $PATH",
		"```bash\necho $x$\n```\n",
		"Run `$x$` and ``a $y$ b``.",
	} {
		if got := renderTeX(in); got != in {
			t.Errorf("renderTeX(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestPlainRenderHasNoANSI(t *testing.T) {
	var b bytes.Buffer
	md := "# Title\n\nSome **bold** text and `code`.\n\n```python\nprint(1)\n```\n"
	if err := render(&b, md, false, 80); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "\033") {
		t.Fatalf("non-TTY output contains ANSI escapes: %q", b.String())
	}
}

func TestTTYRenderIsTruecolour(t *testing.T) {
	var b bytes.Buffer
	if err := render(&b, "```python\nprint(1)\n```\n", true, 80); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "38;2;") {
		t.Fatalf("code block not in truecolour: %q", b.String())
	}
}
