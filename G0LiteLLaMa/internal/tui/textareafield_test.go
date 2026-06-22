package tui

import (
	"strings"
	"testing"
)

func TestTextAreaFieldSetValue(t *testing.T) {
	f := NewTextAreaField(40, 5)
	f.SetValue("hello world")
	if f.Value() != "hello world" {
		t.Fatalf("Value=%q, want %q", f.Value(), "hello world")
	}
}

func TestTextAreaFieldViewClipped(t *testing.T) {
	f := NewTextAreaField(40, 3)
	f.SetValue("line1\nline2\nline3\nline4\nline5")
	view := f.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Fatalf("View returned %d lines, want 3 (MaxVisibleHeight)", len(lines))
	}
}
