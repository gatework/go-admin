package service

import (
	"slices"
	"testing"

	"github.com/gatework/motor-core/logger"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
)

type treeRow struct{ id, parent int }

type treeNode struct {
	id       int
	children []treeNode
}

func buildTreeRows(rows []treeRow, descend func(treeRow) bool) []treeNode {
	return forest(rows,
		func(r treeRow) int { return r.id },
		func(r treeRow) int { return r.parent },
		descend,
		func(r treeRow, children []treeNode) treeNode { return treeNode{id: r.id, children: children} })
}

func ids(nodes []treeNode) []int {
	out := make([]int, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.id)
	}
	return out
}

func TestForestKeepsRowOrderAndNesting(t *testing.T) {
	rows := []treeRow{{1, 0}, {2, 1}, {3, 0}, {4, 1}, {5, 2}, {6, 3}}
	roots := buildTreeRows(rows, always[treeRow])
	if got := ids(roots); !slices.Equal(got, []int{1, 3}) {
		t.Fatalf("roots %v", got)
	}
	if got := ids(roots[0].children); !slices.Equal(got, []int{2, 4}) {
		t.Fatalf("children of 1: %v", got)
	}
	if got := ids(roots[0].children[0].children); !slices.Equal(got, []int{5}) {
		t.Fatalf("children of 2: %v", got)
	}
	if roots[1].children[0].children == nil {
		t.Fatal("a leaf's children must be empty, not nil")
	}
}

func TestForestStopsAtLeavesAndLoops(t *testing.T) {
	rows := []treeRow{{1, 0}, {2, 1}, {3, 2}}
	leafAt2 := buildTreeRows(rows, func(r treeRow) bool { return r.id != 2 })
	if len(leafAt2[0].children[0].children) != 0 {
		t.Fatal("descended below a leaf")
	}
	// 1 -> 2 -> 1 again: the second 1 is attached but not followed.
	looped := []treeRow{{1, 0}, {2, 1}, {1, 2}}
	roots := buildTreeRows(looped, always[treeRow])
	if len(roots) != 1 {
		t.Fatalf("roots %v", ids(roots))
	}
}

// The menu and department trees served to the UI: roots are the rows with no
// parent, children follow the stored order, and a button is always a leaf.
func TestMenuAndDepartmentTreesFromTheDatabase(t *testing.T) {
	db := integrityDB(t)
	for _, m := range []models.SysMenu{
		{MenuID: 1, Title: "System", MenuType: "M", Sort: 1},
		{MenuID: 2, Title: "Users", MenuType: "C", ParentID: 1, Sort: 1},
		{MenuID: 3, Title: "Create user", MenuType: "F", ParentID: 2, Sort: 1},
		{MenuID: 4, Title: "Not under a button", MenuType: "C", ParentID: 3, Sort: 1},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	menus := &Menu{}
	menus.DB, menus.Logger = db, logger.NewHelper(logger.Default())
	labels, err := menus.Labels(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].Label != "System" || labels[0].Children[0].Label != "Users" {
		t.Fatalf("menu labels %+v", labels)
	}
	button := labels[0].Children[0].Children[0]
	if button.Label != "Create user" || len(button.Children) != 0 {
		t.Fatalf("a button got children: %+v", button)
	}

	for _, d := range []models.SysDept{{DeptID: 1, DeptName: "HQ"}, {DeptID: 2, DeptName: "Network", ParentID: 1}, {DeptID: 3, DeptName: "Security", ParentID: 1}} {
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	departments := &Department{}
	departments.DB, departments.Logger = db, logger.NewHelper(logger.Default())
	tree, err := departments.Tree(t.Context(), &dto.DepartmentSearch{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 1 || len(tree[0].Children) != 2 || tree[0].Children[1].Label != "Security" {
		t.Fatalf("department tree %+v", tree)
	}
}

// A role is granted leaf menus; its sidebar needs every directory above them,
// once, and no buttons.
func TestRoleSidebarIncludesAncestorsOnce(t *testing.T) {
	db := integrityDB(t)
	for _, m := range []models.SysMenu{
		{MenuID: 1, Title: "Network", MenuType: "M", Sort: 1},
		{MenuID: 2, Title: "Devices", MenuType: "C", ParentID: 1, Sort: 1},
		{MenuID: 3, Title: "Circuits", MenuType: "C", ParentID: 1, Sort: 2},
		{MenuID: 4, Title: "Export", MenuType: "F", ParentID: 2, Sort: 1},
		{MenuID: 5, Title: "System", MenuType: "M", Sort: 2},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := withAncestors(db, []int{4, 3, 2})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(got))
	for _, m := range got {
		titles = append(titles, m.Title)
	}
	slices.Sort(titles)
	if !slices.Equal(titles, []string{"Circuits", "Devices", "Network"}) {
		t.Fatalf("sidebar menus %v", titles)
	}
}
