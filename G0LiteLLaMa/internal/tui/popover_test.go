package tui

import (
	"strings"
	"testing"

	"g0litellama/internal/tui/shapes"
)

func TestPopoverLayoutBelowAnchor(t *testing.T) {
	p := Popover{Title: "Test", Body: "content", Width: 30, Height: 1}
	anchor := shapes.Rect{Row: 5, Col: 10, Rows: 1, Cols: 30}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 40, Cols: 80}
	p.Layout(anchor, vp)

	if p.Rect.Row != 6 {
		t.Fatalf("popover Row=%d, want 6 (below anchor)", p.Rect.Row)
	}
	if p.Rect.Col != 10 {
		t.Fatalf("popover Col=%d, want 10", p.Rect.Col)
	}
	// Height+3 = 1+3 = 4 rows
	if p.Rect.Rows != 4 {
		t.Fatalf("popover Rows=%d, want 4", p.Rect.Rows)
	}
}

func TestPopoverLayoutAboveAnchorWhenBelowViewport(t *testing.T) {
	p := Popover{Title: "Big", Body: "lots\nof\ncontent", Width: 30, Height: 3}
	anchor := shapes.Rect{Row: 38, Col: 10, Rows: 1, Cols: 30}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 40, Cols: 80}
	p.Layout(anchor, vp)

	// Height+3 = 3+3 = 6 rows
	if p.Rect.Bottom() > vp.Rows {
		t.Fatalf("popover bottom=%d extends past viewport", p.Rect.Bottom())
	}
	// Should be above anchor (anchor.Row - 6 >= 0)
	if p.Rect.Row >= 38 {
		t.Fatalf("popover should be above anchor, got Row=%d", p.Rect.Row)
	}
}

func TestPopoverLayoutClampedToViewport(t *testing.T) {
	p := Popover{Title: "Wide", Body: "wide content", Width: 100, Height: 1}
	anchor := shapes.Rect{Row: 0, Col: -5, Rows: 1, Cols: 100}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 24, Cols: 80}
	p.Layout(anchor, vp)

	if p.Rect.Col < 0 {
		t.Fatalf("popover Col=%d out of viewport", p.Rect.Col)
	}
	if p.Rect.Right() > vp.Cols {
		t.Fatalf("popover right=%d extends past viewport width=%d", p.Rect.Right(), vp.Cols)
	}
	if p.Rect.Row < 0 {
		t.Fatalf("popover Row=%d out of viewport", p.Rect.Row)
	}
}

func TestPopoverContainsPoint(t *testing.T) {
	p := Popover{Title: "T", Body: "body", Width: 20, Height: 1}
	anchor := shapes.Rect{Row: 5, Col: 10, Rows: 1, Cols: 20}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 40, Cols: 80}
	p.Layout(anchor, vp)

	// Inside popover
	if !p.Contains(shapes.Point{Row: p.Rect.Row + 1, Col: p.Rect.Col + 1}) {
		t.Fatal("point inside popover should return true")
	}
	// Outside
	if p.Contains(shapes.Point{Row: 0, Col: 0}) {
		t.Fatal("point outside popover should return false")
	}
}

func TestPopoverCloseHitTarget(t *testing.T) {
	p := Popover{Title: "T", Body: "body", Width: 30, Height: 1}
	anchor := shapes.Rect{Row: 5, Col: 10, Rows: 1, Cols: 30}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 40, Cols: 80}
	p.Layout(anchor, vp)

	hit := p.CloseHitTarget()
	if hit.Action != "popover-close" {
		t.Fatalf("close hit action=%q", hit.Action)
	}
	// Close button should be on title bar row (Rect.Row+1)
	if hit.Rect.Row != p.Rect.Row+1 {
		t.Fatalf("close button row=%d, want %d", hit.Rect.Row, p.Rect.Row+1)
	}
}

func TestPopoverRenderContainsBody(t *testing.T) {
	p := Popover{Title: "My Title", Body: "hello\nworld", Width: 20, Height: 2}
	anchor := shapes.Rect{Row: 0, Col: 0, Rows: 1, Cols: 20}
	vp := shapes.Rect{Row: 0, Col: 0, Rows: 40, Cols: 80}
	p.Layout(anchor, vp)

	rendered := p.Render()
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("rendered missing body text:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[X]") {
		t.Fatalf("rendered missing close button:\n%s", rendered)
	}
}
