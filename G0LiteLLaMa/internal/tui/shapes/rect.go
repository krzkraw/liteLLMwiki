package shapes

// Rect represents a rectangular region of terminal cells.
type Rect struct {
	Row  int
	Col  int
	Rows int // height
	Cols int // width
}

// Contains reports whether p is inside r (inclusive of edges).
func (r Rect) Contains(p Point) bool {
	if p.Row < r.Row || p.Row >= r.Row+r.Rows {
		return false
	}
	if p.Col < r.Col || p.Col >= r.Col+r.Cols {
		return false
	}
	return true
}

// Intersect returns the overlapping region between r and o.
// Returns zero Rect when there is no overlap.
func (r Rect) Intersect(o Rect) Rect {
	tr := r.Row + r.Rows
	br := o.Row + o.Rows
	tc := r.Col + r.Cols
	bc := o.Col + o.Cols

	row := maxInt(r.Row, o.Row)
	col := maxInt(r.Col, o.Col)
	rows := minInt(tr, br) - row
	cols := minInt(tc, bc) - col

	if rows <= 0 || cols <= 0 {
		return Rect{}
	}
	return Rect{Row: row, Col: col, Rows: rows, Cols: cols}
}

// Clamp returns a copy of r whose bounds are clipped inside viewport.
func (r Rect) Clamp(viewport Rect) Rect {
	r.Row = clampInt(r.Row, viewport.Row, viewport.Row+viewport.Rows-1)
	r.Col = clampInt(r.Col, viewport.Col, viewport.Col+viewport.Cols-1)

	if r.Row+r.Rows > viewport.Row+viewport.Rows {
		r.Rows = viewport.Row + viewport.Rows - r.Row
	}
	if r.Col+r.Cols > viewport.Col+viewport.Cols {
		r.Cols = viewport.Col + viewport.Cols - r.Col
	}
	return r
}

// Bottom is the row below the last row of r.
func (r Rect) Bottom() int { return r.Row + r.Rows }

// Right is the column to the right of the last column of r.
func (r Rect) Right() int { return r.Col + r.Cols }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
