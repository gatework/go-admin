// Package paging holds the one rule every list endpoint applies to its page
// parameters.
package paging

// Bounds of a page request.
const (
	DefaultSize = 20
	MaxSize     = 200
	MaxIndex    = 1_000_000
)

// Normalize fills an absent (zero) page index or size with its default and
// reports whether both are then in bounds: index 1..MaxIndex, size 1..MaxSize.
func Normalize(index, size *int) bool {
	if *index == 0 {
		*index = 1
	}
	if *size == 0 {
		*size = DefaultSize
	}
	return *index >= 1 && *index <= MaxIndex && *size >= 1 && *size <= MaxSize
}
