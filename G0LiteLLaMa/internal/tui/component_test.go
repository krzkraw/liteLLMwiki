package tui

import (
	"testing"

	"g0litellama/internal/tui/shapes"
)

func TestHitRegistryAddAndHit(t *testing.T) {
	var r HitRegistry
	r.Add(shapes.Rect{Row: 0, Col: 0, Rows: 5, Cols: 10}, "zone-a")
	r.Add(shapes.Rect{Row: 5, Col: 0, Rows: 5, Cols: 10}, "zone-b")

	if got := r.Hit(2, 3); got != "zone-a" {
		t.Fatalf("Hit(2,3)=%q, want zone-a", got)
	}
	if got := r.Hit(7, 3); got != "zone-b" {
		t.Fatalf("Hit(7,3)=%q, want zone-b", got)
	}
	if got := r.Hit(20, 20); got != "" {
		t.Fatalf("Hit(20,20)=%q, want empty", got)
	}
}

func TestHitRegistryReset(t *testing.T) {
	var r HitRegistry
	r.Add(shapes.Rect{Row: 0, Col: 0, Rows: 5, Cols: 10}, "zone")
	r.Reset()
	if got := r.Hit(2, 3); got != "" {
		t.Fatalf("after reset Hit=%q, want empty", got)
	}
}
