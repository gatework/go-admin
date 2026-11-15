package service

// forest builds the trees rooted at rows whose parent is 0, in one pass over
// rows: they are grouped by parent first, so each node finds its children in
// constant time rather than by rescanning every row. Children keep the order
// of rows. descend reports whether a row's children are attached at all (a
// menu button is always a leaf). A row whose ancestry loops back on itself is
// attached once and not followed again, instead of recursing until the stack
// runs out.
func forest[T, N any](rows []T, id, parent func(T) int, descend func(T) bool, node func(T, []N) N) []N {
	byParent := make(map[int][]T, len(rows))
	for _, row := range rows {
		byParent[parent(row)] = append(byParent[parent(row)], row)
	}
	onPath := make(map[int]bool)
	var build func(T) N
	build = func(row T) N {
		children := make([]N, 0)
		key := id(row)
		if descend(row) && !onPath[key] {
			onPath[key] = true
			for _, child := range byParent[key] {
				children = append(children, build(child))
			}
			delete(onPath, key)
		}
		return node(row, children)
	}
	roots := make([]N, 0, len(byParent[0]))
	for _, row := range byParent[0] {
		roots = append(roots, build(row))
	}
	return roots
}

// always descends into every node.
func always[T any](T) bool { return true }
