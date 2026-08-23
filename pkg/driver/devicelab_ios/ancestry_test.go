package devicelab_ios

import "testing"

// The geometry here is the real shape of the bug: a table occupying the visible
// area, one row inside it, and one row 1900pt below the fold whose children
// report bounds clamped to the table's top edge. Those children are what a
// geometry-only check accepts and a tap then misses.
func clampedTreeFixture() []SnapshotNode {
	parent := func(i int) *int { return &i }
	return []SnapshotNode{
		{Index: 0, Type: "Window", Rect: SnapshotRect{X: 0, Y: 0, Width: 402, Height: 874}},
		{Index: 1, Type: "Table", ParentIndex: parent(0),
			Rect: SnapshotRect{X: 0, Y: 152.333, Width: 402, Height: 659.667}},

		// Genuinely visible row, and its child.
		{Index: 2, Type: "Cell", ParentIndex: parent(1),
			Rect: SnapshotRect{X: 0, Y: 251.333, Width: 402, Height: 43.667}},
		{Index: 3, Type: "StaticText", Label: "Visible", ParentIndex: parent(2),
			Rect: SnapshotRect{X: 0, Y: 251.333, Width: 53, Height: 20.333}},

		// Offscreen row — and a child reporting the table's top edge instead.
		{Index: 4, Type: "Cell", ParentIndex: parent(1),
			Rect: SnapshotRect{X: 0, Y: 2044.333, Width: 402, Height: 44}},
		{Index: 5, Type: "StaticText", Label: "Theme", ParentIndex: parent(4),
			Rect: SnapshotRect{X: 0, Y: 152.333, Width: 53, Height: 20.333}},
	}
}

func TestFrameClampedByOffscreenAncestor(t *testing.T) {
	nodes := clampedTreeFixture()
	const w, h = 402, 874

	t.Run("clamped child of an offscreen row is rejected", func(t *testing.T) {
		if !frameClampedByOffscreenAncestor(nodes, &nodes[5], w, h) {
			t.Error("child of the row at y=2044 should be treated as clamped")
		}
	})

	t.Run("child of a visible row is accepted", func(t *testing.T) {
		if frameClampedByOffscreenAncestor(nodes, &nodes[3], w, h) {
			t.Error("child of the visible row should not be treated as clamped")
		}
	})

	t.Run("the offscreen row itself is rejected via no ancestor", func(t *testing.T) {
		// The row's own rect is honest, so geometry already rejects it; ancestry
		// must not claim otherwise, since the table does overlap the viewport.
		if frameClampedByOffscreenAncestor(nodes, &nodes[4], w, h) {
			t.Error("the row's own frame is truthful; ancestry should stay quiet")
		}
	})
}

func TestFrameClampedByOffscreenAncestor_RefusesWhenUnverifiable(t *testing.T) {
	nodes := clampedTreeFixture()
	const w, h = 402, 874

	t.Run("node from a different snapshot", func(t *testing.T) {
		stale := nodes[5]
		stale.Rect.Y = 999 // same index, different geometry
		if frameClampedByOffscreenAncestor(nodes, &stale, w, h) {
			t.Error("a node that did not come from this snapshot must not be judged")
		}
	})

	t.Run("dangling parent link", func(t *testing.T) {
		orphan := []SnapshotNode{{Index: 0, ParentIndex: func(i int) *int { return &i }(42),
			Rect: SnapshotRect{Width: 10, Height: 10}}}
		if frameClampedByOffscreenAncestor(orphan, &orphan[0], w, h) {
			t.Error("a missing parent must not be treated as offscreen")
		}
	})

	t.Run("parent cycle terminates", func(t *testing.T) {
		a, b := 1, 0
		cyclic := []SnapshotNode{
			{Index: 0, ParentIndex: &a, Rect: SnapshotRect{Width: 10, Height: 10}},
			{Index: 1, ParentIndex: &b, Rect: SnapshotRect{Width: 10, Height: 10}},
		}
		if frameClampedByOffscreenAncestor(cyclic, &cyclic[0], w, h) {
			t.Error("a cycle should bail out rather than report clamping")
		}
	})

	t.Run("unknown screen size", func(t *testing.T) {
		if frameClampedByOffscreenAncestor(nodes, &nodes[5], 0, 0) {
			t.Error("without a viewport there is nothing to contradict")
		}
	})

	t.Run("frameless ancestors are ignored", func(t *testing.T) {
		zero := 0
		tree := []SnapshotNode{
			{Index: 0, Rect: SnapshotRect{}}, // no geometry
			{Index: 1, ParentIndex: &zero, Rect: SnapshotRect{Y: 10, Width: 50, Height: 20}},
		}
		if frameClampedByOffscreenAncestor(tree, &tree[1], w, h) {
			t.Error("a frameless container carries no contradiction")
		}
	})
}

func TestRectOverlapsViewport(t *testing.T) {
	const w, h = 400, 800
	for name, tc := range map[string]struct {
		rect SnapshotRect
		want bool
	}{
		"inside":             {SnapshotRect{X: 10, Y: 10, Width: 100, Height: 50}, true},
		"below the fold":     {SnapshotRect{X: 0, Y: 2044, Width: 400, Height: 44}, false},
		"above the top":      {SnapshotRect{X: 0, Y: -100, Width: 400, Height: 50}, false},
		"straddles the top":  {SnapshotRect{X: 0, Y: -10, Width: 400, Height: 50}, true},
		"off to the right":   {SnapshotRect{X: 400, Y: 10, Width: 50, Height: 50}, false},
		"taller than screen": {SnapshotRect{X: 0, Y: 0, Width: 400, Height: 4000}, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := rectOverlapsViewport(tc.rect, w, h); got != tc.want {
				t.Errorf("rectOverlapsViewport(%+v) = %v, want %v", tc.rect, got, tc.want)
			}
		})
	}
}
