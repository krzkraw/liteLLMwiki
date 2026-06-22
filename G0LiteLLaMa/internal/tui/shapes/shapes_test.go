package shapes

import "testing"

func TestRectContains(t *testing.T) {
	r := Rect{Row: 2, Col: 3, Rows: 5, Cols: 10}
	tests := []struct {
		p    Point
		want bool
	}{
		{Point{2, 3}, true},
		{Point{6, 12}, true},
		{Point{6, 13}, false},
		{Point{7, 12}, false},
		{Point{1, 3}, false},
		{Point{2, 2}, false},
	}
	for _, tc := range tests {
		got := r.Contains(tc.p)
		if got != tc.want {
			t.Errorf("Rect{2,3,5,10}.Contains(%+v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

func TestRectIntersect(t *testing.T) {
	a := Rect{Row: 0, Col: 0, Rows: 10, Cols: 20}
	b := Rect{Row: 5, Col: 5, Rows: 10, Cols: 20}
	got := a.Intersect(b)
	want := Rect{Row: 5, Col: 5, Rows: 5, Cols: 15}
	if got != want {
		t.Errorf("Intersect = %+v, want %+v", got, want)
	}

	// no overlap
	c := Rect{Row: 20, Col: 20, Rows: 5, Cols: 5}
	got = a.Intersect(c)
	if got != (Rect{}) {
		t.Errorf("no-overlap Intersect = %+v, want zero", got)
	}
}

func TestRectClamp(t *testing.T) {
	vp := Rect{Row: 0, Col: 0, Rows: 24, Cols: 80}
	r := Rect{Row: -2, Col: 10, Rows: 30, Cols: 100}
	got := r.Clamp(vp)
	if got.Row < 0 || got.Col < 0 {
		t.Errorf("clamp produced negative: %+v", got)
	}
	if got.Row+got.Rows > vp.Rows {
		t.Errorf("clamp spills bottom: %+v", got)
	}
	if got.Col+got.Cols > vp.Cols {
		t.Errorf("clamp spills right: %+v", got)
	}
}

func TestRectBottomRight(t *testing.T) {
	r := Rect{Row: 3, Col: 5, Rows: 7, Cols: 9}
	if r.Bottom() != 10 {
		t.Errorf("Bottom = %d, want 10", r.Bottom())
	}
	if r.Right() != 14 {
		t.Errorf("Right = %d, want 14", r.Right())
	}
}
