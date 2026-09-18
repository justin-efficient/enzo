package ghclient

// Node is an issue positioned in the sub-issue tree.
type Node struct {
	Issue Issue
	// Depth is 0 for a root issue, 1 for its sub-issues, and so on.
	Depth int
}

// Arrange orders issues so sub-issues follow their parent, indented one level
// deeper. repo is "owner/name" for the repository the issues came from.
//
// Only parents that are themselves in the list can nest a child: an issue
// whose parent is missing — not assigned to you, closed, or in another
// repository — stays at the top level rather than disappearing.
//
// Input order is otherwise preserved, so the caller's sort still decides which
// roots come first and in what order siblings appear.
func Arrange(repo string, issues []Issue) []Node {
	children := make(map[int][]Issue, len(issues))
	present := make(map[int]bool, len(issues))
	for _, iss := range issues {
		present[iss.Number] = true
	}

	var roots []Issue
	for _, iss := range issues {
		if parentOf(repo, iss, present) {
			children[iss.ParentNumber] = append(children[iss.ParentNumber], iss)
			continue
		}
		roots = append(roots, iss)
	}

	out := make([]Node, 0, len(issues))
	// visiting guards against a parent chain that loops back on itself, which
	// would otherwise recurse forever.
	visiting := make(map[int]bool, len(issues))

	var walk func(iss Issue, depth int)
	walk = func(iss Issue, depth int) {
		if visiting[iss.Number] {
			return
		}
		visiting[iss.Number] = true
		defer delete(visiting, iss.Number)

		out = append(out, Node{Issue: iss, Depth: depth})
		for _, child := range children[iss.Number] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}

	// A cycle leaves its members unreachable from any root; show them flat
	// rather than dropping them off the list entirely.
	if len(out) < len(issues) {
		seen := make(map[int]bool, len(out))
		for _, n := range out {
			seen[n.Issue.Number] = true
		}
		for _, iss := range issues {
			if !seen[iss.Number] {
				out = append(out, Node{Issue: iss})
			}
		}
	}
	return out
}

// parentOf reports whether iss should nest under a parent in the same list.
func parentOf(repo string, iss Issue, present map[int]bool) bool {
	if !iss.HasParent() || iss.ParentRepo != repo {
		return false
	}
	// A parent cannot be itself.
	if iss.ParentNumber == iss.Number {
		return false
	}
	return present[iss.ParentNumber]
}

// Issues strips the tree back down to a flat slice, in display order.
func Issues(nodes []Node) []Issue {
	out := make([]Issue, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Issue)
	}
	return out
}
