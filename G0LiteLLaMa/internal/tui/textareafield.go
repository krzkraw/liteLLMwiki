package tui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
)

// TextAreaField is a bounded multiline text editor wrapping Bubbles textarea.
// MaxVisibleHeight controls how many lines are shown — content beyond that
// is clipped.
type TextAreaField struct {
	inner            textarea.Model
	MaxWidth         int
	MaxVisibleHeight int
}

// NewTextAreaField creates a bounded textarea with the given max width and
// visible height. The inner model is ready for Focus and Update.
func NewTextAreaField(width, visibleHeight int) TextAreaField {
	m := textarea.New()
	m.CharLimit = 0
	m.MaxWidth = width
	m.ShowLineNumbers = false
	return TextAreaField{
		inner:            m,
		MaxWidth:         width,
		MaxVisibleHeight: visibleHeight,
	}
}

// Value returns the current text content.
func (f *TextAreaField) Value() string { return f.inner.Value() }

// SetValue replaces the current text content.
func (f *TextAreaField) SetValue(v string) { f.inner.SetValue(v) }

// Focus focuses the inner textarea.
func (f *TextAreaField) Focus() tea.Cmd { return f.inner.Focus() }

// Blur blurs the inner textarea.
func (f *TextAreaField) Blur() { f.inner.Blur() }

// Focused reports whether the field is focused.
func (f *TextAreaField) Focused() bool { return f.inner.Focused() }

// Update delegates to the inner textarea.
func (f *TextAreaField) Update(msg tea.Msg) (TextAreaField, tea.Cmd) {
	var cmd tea.Cmd
	f.inner, cmd = f.inner.Update(msg)
	return *f, cmd
}

// View returns the rendered text, clipped to MaxVisibleHeight lines.
func (f TextAreaField) View() string {
	rendered := f.inner.View()
	lines := strings.Split(rendered, "\n")
	if len(lines) > f.MaxVisibleHeight {
		lines = lines[:f.MaxVisibleHeight]
	}
	return strings.Join(lines, "\n")
}
