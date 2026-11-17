package main

import "testing"

func TestGORMSaveIsReported(t *testing.T) {
	root := fixture(t, map[string]string{"save.go": `package sample
import "gorm.io/gorm"
type row struct{ ID int }
func f(db *gorm.DB, tx *gorm.DB, r *row) {
	db.Save(r)
	_ = tx.Model(r).Save(r).Error
	db.Create(r)
}
`, "pkg.go": `package sample
import "example.com/connection"
func g() { connection.Save(nil) }
`})
	if findings := only(t, check(t, root, options{}), checkGORMSave); len(findings) != 2 {
		t.Fatalf("got %v, want the two GORM Save calls", findings)
	}
}
