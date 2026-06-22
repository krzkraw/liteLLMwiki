package shapes

// LayoutResult holds the computed placement of a primitive inside a viewport.
type LayoutResult struct {
	Content Rect // the region the primitive occupies
	Scroll  Rect // the region reserved for a scrollbar (zero if none)
}
