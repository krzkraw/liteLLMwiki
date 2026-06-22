package tui

import (
	"strings"
	"testing"
)

func TestScrollBoxPinnedAutoFollow(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Pinned: true}

	// Start empty, no content.
	s.SetLines(nil)
	if s.Offset != 0 {
		t.Fatalf("empty pinned Offset=%d, want 0", s.Offset)
	}

	// Add content, should auto-follow to bottom.
	s.SetLines(strLines(0, 10))
	if s.Offset != 5 {
		t.Fatalf("10 lines pinned Offset=%d, want 5", s.Offset)
	}
}

func TestScrollBoxUnpinnedStaysPut(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 20), Offset: 3, Pinned: false}

	// Add more lines without pin.
	s.SetLines(strLines(0, 30))
	if s.Offset != 3 {
		t.Fatalf("unpinned Offset changed to %d, want 3", s.Offset)
	}
}

func TestScrollBoxScrollUpDetachesPin(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10), Offset: 5, Pinned: true}

	s.ScrollUp(1)
	if s.Pinned {
		t.Fatal("ScrollUp should detach pin")
	}
	if s.Offset != 5 {
		t.Fatalf("after ScrollUp Offset=%d, want 5 (clamped to max)", s.Offset)
	}
}

func TestScrollBoxScrollDownReengagesPin(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10), Offset: 5, Pinned: false}

	s.ScrollDown(1)
	if !s.Pinned {
		t.Fatal("ScrollDown to bottom should re-engage pin")
	}
	if s.Offset != 5 {
		t.Fatalf("after ScrollDown Offset=%d, want 5", s.Offset)
	}
}

func TestScrollBoxMaxOffset(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10)}
	if want := 5; s.MaxOffset() != want {
		t.Fatalf("MaxOffset=%d, want %d", s.MaxOffset(), want)
	}

	s.ViewLines = 10
	if want := 0; s.MaxOffset() != want {
		t.Fatalf("MaxOffset=%d, want %d (content fits)", s.MaxOffset(), want)
	}
}

func TestScrollBoxClamp(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10), Offset: 999}
	s.ClampOffset()
	if s.Offset != 5 {
		t.Fatalf("clamp Offset=%d, want 5", s.Offset)
	}

	s.Offset = -1
	s.ClampOffset()
	if s.Offset != 0 {
		t.Fatalf("clamp negative Offset=%d, want 0", s.Offset)
	}
}

func TestScrollBoxVisibleLines(t *testing.T) {
	s := ScrollBox{ViewLines: 3, Lines: strLines(0, 10), Offset: 4}
	visible := s.VisibleLines()
	if len(visible) != 3 {
		t.Fatalf("VisibleLines count=%d, want 3", len(visible))
	}
	if !strings.Contains(visible[0], "4") {
		t.Fatalf("first visible line should be 4, got %q", visible[0])
	}
}

func TestScrollBoxViewContainsScrollbar(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10)}
	rendered := s.View(40)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("View returned %d lines, want 5", len(lines))
	}
	// Each line should be content width + scrollbar column = 41 chars rendered
	for i, line := range lines {
		if len(line) < 40 {
			t.Errorf("line %d too short: %d chars", i, len(line))
		}
	}
}

func TestScrollBoxContentBackgroundPadsEmptyCells(t *testing.T) {
	s := ScrollBox{ViewLines: 3, Lines: []string{"x"}, ContentBg: "24"}

	view := s.View(6)
	if !strings.Contains(view, "\x1b[48;5;24m") {
		t.Fatalf("expected content background padding in:\n%q", view)
	}
}

func TestScrollBoxHitTargets(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 10)}
	targets := s.HitTargets(10, 5)
	if len(targets) != 2 {
		t.Fatalf("HitTargets count=%d, want 2", len(targets))
	}
	if targets[0].Action != "scrollbar-track" {
		t.Fatalf("first target action=%q, want scrollbar-track", targets[0].Action)
	}
	if targets[1].Action != "scrollbar-thumb" {
		t.Fatalf("second target action=%q, want scrollbar-thumb", targets[1].Action)
	}
}

func TestScrollBoxScrollToBottom(t *testing.T) {
	s := ScrollBox{ViewLines: 5, Lines: strLines(0, 20), Offset: 0, Pinned: false}
	s.ScrollToBottom()
	if !s.Pinned {
		t.Fatal("ScrollToBottom should set Pinned=true")
	}
	if s.Offset != 15 {
		t.Fatalf("ScrollToBottom Offset=%d, want 15", s.Offset)
	}
}

func strLines(start, end int) []string {
	var lines []string
	for i := start; i < end; i++ {
		lines = append(lines, "line "+string(rune('0'+i%10)))
	}
	return lines
}
