package cliapp

import (
	"bytes"
	"testing"
)

func TestStylerColorsOnlyWhenEnabled(t *testing.T) {
	colored := styler{color: true}
	if got, want := colored.green("enabled"), "\x1b[32menabled\x1b[0m"; got != want {
		t.Fatalf("green = %q, want %q", got, want)
	}
	if got, want := colored.ok("Created"), "\x1b[32m✓\x1b[0m Created"; got != want {
		t.Fatalf("ok = %q, want %q", got, want)
	}

	plain := styler{}
	if got := plain.bold("name"); got != "name" {
		t.Fatalf("plain bold = %q, want %q", got, "name")
	}
	if got := plain.ok("Created"); got != "✓ Created" {
		t.Fatalf("plain ok = %q, want %q", got, "✓ Created")
	}
}

func TestNewStylerStaysPlainForNonTerminalWriters(t *testing.T) {
	if newStyler(&bytes.Buffer{}).color {
		t.Fatal("styler colors a plain buffer")
	}
}
