package main

import (
	"os/exec"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	sup = scriptTable("0123456789+-=()abcdefghijklmnoprstuvwxyzABDEGHIJKLMNOPRTUVW",
		"⁰¹²³⁴⁵⁶⁷⁸⁹⁺⁻⁼⁽⁾ᵃᵇᶜᵈᵉᶠᵍʰⁱʲᵏˡᵐⁿᵒᵖʳˢᵗᵘᵛʷˣʸᶻᴬᴮᴰᴱᴳᴴᴵᴶᴷᴸᴹᴺᴼᴾᴿᵀᵁⱽᵂ")
	sub = scriptTable("0123456789+-=()aehijklmnoprstuvx", "₀₁₂₃₄₅₆₇₈₉₊₋₌₍₎ₐₑₕᵢⱼₖₗₘₙₒₚᵣₛₜᵤᵥₓ")
	big = map[string]string{"int": "∫", "iint": "∬", "oint": "∮", "sum": "∑", "prod": "∏"}

	limitsRe  = regexp.MustCompile(`\\(no)?limits`)
	fracRe    = regexp.MustCompile(`\\[dt]?frac\{([^{}]*)\}\{([^{}]*)\}`)
	sqrtRe    = regexp.MustCompile(`\\sqrt\{([^{}]*)\}`)
	scriptRe  = regexp.MustCompile(`([_^])(\{[^{}]*\}|\\[a-zA-Z]+|.)`)
	commandRe = regexp.MustCompile(`\\[a-zA-Z]+(?:\{[^{}]*\})?`)
	displayRe = regexp.MustCompile(`(?s)\$\$(.+?)\$\$|\\\[(.+?)\\\]`)
	fenceRe   = regexp.MustCompile("^\\s*(```+|~~~+)")

	utftexPath, _ = exec.LookPath("utftex")
)

// Glyphs that are already scripts map to themselves, so nested scripts
// (e^{-x^2}) stay raised.
func scriptTable(from, to string) map[rune]rune {
	t := map[rune]rune{}
	f, g := []rune(from), []rune(to)
	for i := range f {
		t[f[i]] = g[i]
	}
	for _, r := range g {
		t[r] = r
	}
	return t
}

func utftex(expr string) (string, bool) {
	if utftexPath == "" {
		return "", false
	}
	b, err := exec.Command(utftexPath, expr).Output()
	out := strings.TrimRight(string(b), "\n")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", false
	}
	return out, true
}

func symbol(cmd string) string {
	if s, ok := big[cmd[1:]]; ok {
		return s
	}
	if out, ok := utftex(cmd); ok && !strings.Contains(out, "\n") {
		return out
	}
	return cmd
}

// flat writes maths on one line: utftex draws every script on its own line,
// so scripts become Unicode super/subscripts where they exist and ^(...)
// otherwise.
func flat(expr string) string {
	expr = limitsRe.ReplaceAllString(expr, "")
	// The patterns only match brace pairs with nothing nested inside, so repeat
	// until stable: \frac{-b \pm \sqrt{x}}{2a} needs the \sqrt flattened first.
	for prev := ""; prev != expr; {
		prev = expr
		expr = sqrtRe.ReplaceAllString(expr, "√($1)")
		expr = fracRe.ReplaceAllString(expr, "($1)/($2)")
	}
	expr = scriptRe.ReplaceAllStringFunc(expr, func(m string) string {
		g := scriptRe.FindStringSubmatch(m)
		mark, body := g[1], g[2]
		if strings.HasPrefix(body, "{") {
			body = body[1 : len(body)-1]
		}
		body = flat(body)
		table := sup
		if mark == "_" {
			table = sub
		}
		var b strings.Builder
		for _, r := range body {
			s, ok := table[r]
			if !ok {
				if utf8.RuneCountInString(body) == 1 {
					return mark + body
				}
				return mark + "(" + body + ")"
			}
			b.WriteRune(s)
		}
		return b.String()
	})
	expr = commandRe.ReplaceAllStringFunc(expr, symbol)
	return strings.NewReplacer("{", "", "}", "", `\,`, " ", `\ `, " ").Replace(expr)
}

func inlineMath(expr string) string {
	if out, ok := utftex(expr); ok && !strings.Contains(out, "\n") {
		return out
	}
	return flat(expr)
}

func isWord(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// matchInline finds \(...\) or $...$ starting exactly at i. The $ form
// follows Pandoc: no space just inside the dollars, the opening $ not after a
// backslash, $ or word character, the closing one not before a digit or $, so
// "$5 and $10" and shell "$HOME" stay as they are. Go's regexp has no
// lookaround, hence the hand-written scan.
func matchInline(s string, i int) (end int, expr string, ok bool) {
	if strings.HasPrefix(s[i:], `\(`) {
		start := i + 2
		if start < len(s) && s[start] != '\n' {
			_, w := utf8.DecodeRuneInString(s[start:])
			for j := start + w; j < len(s); j++ {
				if s[j] == '\n' {
					break
				}
				if strings.HasPrefix(s[j:], `\)`) {
					return j + 2, s[start:j], true
				}
			}
		}
		return 0, "", false
	}
	if s[i] != '$' {
		return 0, "", false
	}
	if i > 0 {
		prev, _ := utf8.DecodeLastRuneInString(s[:i])
		if prev == '\\' || prev == '$' || isWord(prev) {
			return 0, "", false
		}
	}
	first, _ := utf8.DecodeRuneInString(s[i+1:])
	if i+1 >= len(s) || unicode.IsSpace(first) {
		return 0, "", false
	}
	j := strings.IndexByte(s[i+1:], '$')
	if j <= 0 {
		return 0, "", false
	}
	j += i + 1
	body := s[i+1 : j]
	if strings.Contains(body, "\n") {
		return 0, "", false
	}
	if last, _ := utf8.DecodeLastRuneInString(body); unicode.IsSpace(last) {
		return 0, "", false
	}
	if next, _ := utf8.DecodeRuneInString(s[j+1:]); j+1 < len(s) && (next == '$' || unicode.IsDigit(next)) {
		return 0, "", false
	}
	return j + 1, body, true
}

func replaceInline(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if end, expr, ok := matchInline(s, i); ok {
			if out := inlineMath(expr); out != "" {
				b.WriteString(out)
			} else {
				b.WriteString(s[i:end])
			}
			i = end
			continue
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+w])
		i += w
	}
	return b.String()
}

func replaceDisplay(s string) string {
	return displayRe.ReplaceAllStringFunc(s, func(m string) string {
		g := displayRe.FindStringSubmatch(m)
		expr := g[1]
		if expr == "" {
			expr = g[2]
		}
		if out, ok := utftex(strings.TrimSpace(expr)); ok {
			return "\n```text\n" + out + "\n```\n"
		}
		return m
	})
}

func texOutside(s string) string {
	return replaceInline(replaceDisplay(s))
}

// Matches (`+)(.+?)\1 by hand: the closing run must equal the opening one,
// a backreference Go's regexp lacks.
func codeSpan(s string, from int) (start, end int, ok bool) {
	for i := from; i < len(s); i++ {
		if s[i] != '`' {
			continue
		}
		n := 0
		for i+n < len(s) && s[i+n] == '`' {
			n++
		}
		for k := n; k >= 1; k-- {
			bodyStart := i + k
			if bodyStart >= len(s) {
				continue
			}
			_, w := utf8.DecodeRuneInString(s[bodyStart:])
			if j := strings.Index(s[bodyStart+w:], strings.Repeat("`", k)); j >= 0 {
				return i, bodyStart + w + j + k, true
			}
		}
	}
	return 0, 0, false
}

func texOutsideCodeSpans(s string) string {
	var b strings.Builder
	last := 0
	for {
		start, end, ok := codeSpan(s, last)
		if !ok {
			break
		}
		b.WriteString(texOutside(s[last:start]))
		b.WriteString(s[start:end])
		last = end
	}
	b.WriteString(texOutside(s[last:]))
	return b.String()
}

func splitLinesKeepEnds(s string) []string {
	var lines []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

func renderTeX(md string) string {
	if utftexPath == "" {
		return md
	}
	var out, buf strings.Builder
	fence := ""
	for _, line := range splitLinesKeepEnds(md) {
		switch m := fenceRe.FindStringSubmatch(line); {
		case fence == "" && m != nil:
			out.WriteString(texOutsideCodeSpans(buf.String()))
			buf.Reset()
			buf.WriteString(line)
			fence = m[1]
		case fence != "":
			buf.WriteString(line)
			if strings.HasPrefix(strings.TrimSpace(line), fence) {
				out.WriteString(buf.String())
				buf.Reset()
				fence = ""
			}
		default:
			buf.WriteString(line)
		}
	}
	if fence != "" {
		out.WriteString(buf.String())
	} else {
		out.WriteString(texOutsideCodeSpans(buf.String()))
	}
	return out.String()
}
