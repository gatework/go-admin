// Package csvsafe keeps exported CSV cells from being run as spreadsheet
// formulas.
package csvsafe

import (
	"strings"
	"unicode"
)

// Cell returns value ready for a CSV export. A value a spreadsheet would read
// as a formula (one starting, after leading spaces, with = + - or @) or one
// carrying a tab or line break is prefixed with an apostrophe, so it is shown
// as text and never evaluated.
func Cell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	if strings.ContainsAny(value, "\t\r\n") {
		return "'" + value
	}
	return value
}
