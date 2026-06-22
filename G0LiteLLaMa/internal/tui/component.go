package tui

import "g0litellama/internal/tui/shapes"

// HitTarget registers a clickable region with an action identifier.
// The action string is passed to the handler so Model can dispatch behavior
// without parsing rendered text.
type HitTarget struct {
	Rect   shapes.Rect
	Action string // e.g. "scrollbar-up", "popover-close", "sample-value"
}

// HitRegistry is a set of registered clickable regions.
// Zero value is ready to use.
type HitRegistry struct {
	targets []HitTarget
}

func (r *HitRegistry) Add(rect shapes.Rect, action string) {
	r.targets = append(r.targets, HitTarget{Rect: rect, Action: action})
}

func (r *HitRegistry) Reset() {
	r.targets = r.targets[:0]
}

// Hit returns the action string for the first target that contains p,
// or empty string if no target matches.
func (r *HitRegistry) Hit(row, col int) string {
	p := shapes.Point{Row: row, Col: col}
	for _, t := range r.targets {
		if t.Rect.Contains(p) {
			return t.Action
		}
	}
	return ""
}
