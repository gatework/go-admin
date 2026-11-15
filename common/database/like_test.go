package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLikeContainsMatchesWildcardsLiterally(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ Name string }
	if err := db.AutoMigrate(&row{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"100% done", "1000 done", "a_b", "axb", "x!y"} {
		if err := db.Create(&row{Name: name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for keyword, want := range map[string][]string{
		"0%": {"100% done"},
		"a_": {"a_b"},
		"!":  {"x!y"},
		"b":  {"a_b", "axb"},
	} {
		var got []string
		if err := db.Model(&row{}).Where("name LIKE ? "+LikeEscape, LikeContains(keyword)).Order("name").Pluck("name", &got).Error; err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Errorf("%q matched %v, want %v", keyword, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q matched %v, want %v", keyword, got, want)
			}
		}
	}
}
