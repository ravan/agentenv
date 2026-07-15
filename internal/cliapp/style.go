package cliapp

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Nerd Font glyphs used to decorate command output. They are plain Unicode
// characters, so they survive pipes and only need a patched font to render.
const (
	iconProfile   = "" // nf-fa-cube
	iconRtk       = "" // nf-fa-bolt
	iconTokensave = "" // nf-fa-share_alt
	iconProxy     = "" // nf-fa-plug
	iconSkills    = "" // nf-fa-book
)

// integrationIcons decorates each helper tool in summaries.
var integrationIcons = map[string]string{
	"rtk":       iconRtk,
	"tokensave": iconTokensave,
}

// styler colors output destined for an interactive terminal and leaves it
// plain for pipes, redirects, and tests. NO_COLOR and TERM=dumb disable
// coloring even on a terminal.
type styler struct{ color bool }

func newStyler(writer io.Writer) styler {
	file, ok := writer.(*os.File)
	if !ok {
		return styler{}
	}
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return styler{}
	}
	return styler{color: term.IsTerminal(int(file.Fd()))}
}

func (s styler) paint(code, text string) string {
	if !s.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s styler) bold(text string) string  { return s.paint("1", text) }
func (s styler) dim(text string) string   { return s.paint("2", text) }
func (s styler) green(text string) string { return s.paint("32", text) }
func (s styler) cyan(text string) string  { return s.paint("36", text) }

// ok prefixes a confirmation message with a green check mark.
func (s styler) ok(message string) string {
	return s.green("✓") + " " + message
}
