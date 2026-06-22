package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"g0litellama/internal/tui/shapes"
)

// Popover renders a bounded overlay panel anchored to a source rect.
// Zero value is not ready to use; call Layout first.
type Popover struct {
	Title  string
	Body   string   // rendered body (caller builds this)
	Width  int      // desired width
	Height int      // number of body lines

	// computed by Layout
	Rect shapes.Rect // final position and size inside the viewport
}

// Layout computes the popover position anchored below the anchor rect,
// clamped inside the viewport. Call after setting Title, Body lines, Width.
func (p *Popover) Layout(anchor shapes.Rect, viewport shapes.Rect) {
	p.Width = clampInt(p.Width, 20, viewport.Cols-4)

	// Rendered height = top border (1) + title bar (1) + body + bottom border (1)
	r := shapes.Rect{
		Row:  anchor.Bottom(),
		Col:  anchor.Col,
		Rows: p.Height + 3,
		Cols: p.Width + 2,
	}

	// If it would extend below viewport, place above anchor instead.
	if r.Bottom() > viewport.Bottom() && anchor.Row-r.Rows >= viewport.Row {
		r.Row = anchor.Row - r.Rows
	}

	// Clamp inside viewport.
	r = r.Clamp(viewport)

	p.Rect = r
}

// Contains reports whether p is inside the popover's bounding rect.
func (p *Popover) Contains(point shapes.Point) bool {
	return p.Rect.Contains(point)
}

// CloseHitTarget returns a hit target for the close button at the top-right
// of the popover. The action string is "popover-close".
func (p *Popover) CloseHitTarget() HitTarget {
	// [X] is on the title bar (row 1 of the rendered popover).
	return HitTarget{
		Rect: shapes.Rect{
			Row:  p.Rect.Row + 1,
			Col:  p.Rect.Right() - 5,
			Rows: 1,
			Cols: 5,
		},
		Action: "popover-close",
	}
}

// Apply overlays the rendered popover onto the base view string.
func (p *Popover) Apply(base string) string {
	if p.Rect.Rows <= 0 || p.Rect.Cols <= 0 {
		return base
	}
	block := p.Render()
	return overlayBlock(base, block, p.Rect.Row, p.Rect.Col)
}

// Render returns the styled popover block (border + title + body).
func (p *Popover) Render() string {
	title := p.Title
	if title == "" {
		title = "Popup"
	}
	// Title line with close indicator
	innerW := p.Width
	titleBar := " " + truncateToWidth(title, innerW-4) + " [X]"
	titleBar = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Width(innerW).
		Render(titleBar)

	topBorder := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Border(lipgloss.NormalBorder(), true, true, false, true).
		Width(innerW).
		Render("")
	botBorder := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Border(lipgloss.NormalBorder(), false, true, true, true).
		Width(innerW).
		Render("")

	bodyLines := strings.Split(p.Body, "\n")
	padded := make([]string, 0, p.Height)
	for _, line := range bodyLines {
		padded = append(padded, lipgloss.NewStyle().
			Width(innerW).
			MaxWidth(innerW).
			Render(line))
	}
	// Fill remaining
	for len(padded) < p.Height {
		padded = append(padded, strings.Repeat(" ", innerW))
	}

	all := append([]string{topBorder, titleBar}, padded...)
	all = append(all, botBorder)
	return strings.Join(all, "\n")
}

// overlayBlock overlays block string onto base at given (row, col).
// This replaces the old top-level overlayBlock function (now a Popover helper).
func overlayBlock(base string, block string, row int, column int) string {
	lines := strings.Split(base, "\n")
	blockLines := strings.Split(block, "\n")
	if len(blockLines) == 0 {
		return base
	}
	shadow := lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(strings.Repeat(" ", lipgloss.Width(firstRenderedLine(block))))
	for index := range blockLines {
		shadowRow := row + index + 1
		if shadowRow >= 0 && shadowRow < len(lines) {
			lines[shadowRow] = overlayLine(lines[shadowRow], shadow, column+2)
		}
	}
	for index, blockLine := range blockLines {
		targetRow := row + index
		if targetRow >= 0 && targetRow < len(lines) {
			lines[targetRow] = overlayLine(lines[targetRow], blockLine, column)
		}
	}
	return strings.Join(lines, "\n")
}

// overlayLine is the line-level overlay used by overlayBlock.
func overlayLine(base string, block string, column int) string {
	if column < 0 {
		column = 0
	}
	baseWidth := ansi.StringWidth(base)
	prefix := ansi.Cut(base, 0, minInt(column, baseWidth))
	for ansi.StringWidth(prefix) < column {
		prefix += " "
	}
	blockWidth := lipgloss.Width(block)
	suffix := ""
	suffixStart := column + blockWidth
	if suffixStart < baseWidth {
		suffix = ansi.Cut(base, suffixStart, baseWidth)
	}
	return prefix + block + suffix
}
