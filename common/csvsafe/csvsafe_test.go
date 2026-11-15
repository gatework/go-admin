package csvsafe

import "testing"

func TestCell(t *testing.T) {
	for value, want := range map[string]string{
		"core-01":           "core-01",
		"=HYPERLINK(\"x\")": "'=HYPERLINK(\"x\")",
		"  +1":              "'  +1",
		"-2":                "'-2",
		"@SUM(A1)":          "'@SUM(A1)",
		"line\nbreak":       "'line\nbreak",
		"tab\tseparated":    "'tab\tseparated",
		"":                  "",
		"10.0.0.1":          "10.0.0.1",
	} {
		if got := Cell(value); got != want {
			t.Errorf("Cell(%q) = %q, want %q", value, got, want)
		}
	}
}
