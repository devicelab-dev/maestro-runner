package devicelab_ios

// iOS reports some frames pre-clipped to the viewport — Flutter semantics
// especially — so a 12pt sliver of an 80pt row sitting past the fold publishes
// bounds flush against its scroll container's visible edge and reads as fully
// visible. The tap that follows lands in the edge zone and no-ops.
//
// The geometry check cannot see this, because the frame it is handed is already
// a lie. The ancestors are not: a row 1900pt below the fold still reports its
// real offscreen rect while clamping what it publishes for its children. So the
// contradiction — a descendant claiming to be on screen underneath an ancestor
// that is wholly off it — is the signal, and one pass over the parent chain
// finds it without spending an extra scroll to disambiguate.

// frameClampedByOffscreenAncestor reports whether any ancestor of node lies
// wholly outside the viewport, which makes node's own bounds untrustworthy.
//
// It returns false whenever ancestry cannot be established — a node that did not
// come from this snapshot, a broken parent link, a cycle — so an unverifiable
// tree never blocks a legitimate match.
func frameClampedByOffscreenAncestor(nodes []SnapshotNode, node *SnapshotNode, screenW, screenH int) bool {
	if node == nil || len(nodes) == 0 || screenW <= 0 || screenH <= 0 {
		return false
	}

	byIndex := make(map[int]*SnapshotNode, len(nodes))
	for i := range nodes {
		byIndex[nodes[i].Index] = &nodes[i]
	}

	// Confirm the node really came from this snapshot before trusting its
	// parent links: findElement can return a match from an earlier tree, and
	// stale indices would point at unrelated ancestors.
	self, ok := byIndex[node.Index]
	if !ok || self.Rect != node.Rect {
		return false
	}

	seen := map[int]bool{self.Index: true}
	for cur := self; cur.ParentIndex != nil; {
		parentIndex := *cur.ParentIndex
		if seen[parentIndex] {
			return false // malformed chain; refuse to loop
		}
		seen[parentIndex] = true

		parent, ok := byIndex[parentIndex]
		if !ok {
			return false
		}
		// Frameless container nodes carry no geometry to contradict anything.
		if parent.Rect.Width > 0 && parent.Rect.Height > 0 &&
			!rectOverlapsViewport(parent.Rect, screenW, screenH) {
			return true
		}
		cur = parent
	}
	return false
}

// rectOverlapsViewport reports whether r shares any area with the screen.
func rectOverlapsViewport(r SnapshotRect, screenW, screenH int) bool {
	return r.X < float64(screenW) && r.Y < float64(screenH) &&
		r.X+r.Width > 0 && r.Y+r.Height > 0
}
