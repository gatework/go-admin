package delivery

import (
	"testing"
	"unicode/utf8"
)

func TestAppendCappedNeverSplitsACharacter(t *testing.T) {
	for _, tc := range []struct {
		output, part string
		limit        int
		want         string
		full         bool
	}{
		{"ab", "cd", 10, "abcd", true},
		{"ab", "cd", 4, "abcd", true},
		{"ab", "cd", 3, "abc", false},
		{"ab", "接口", 6, "ab接", false}, // 接 is 3 bytes; 口 would need 3 more
		{"ab", "接口", 7, "ab接", false},
		{"ab", "接口", 4, "ab", false},
		{"abcd", "x", 3, "abcd", false},
	} {
		got, full := appendCapped(tc.output, tc.part, tc.limit)
		if got != tc.want || full != tc.full || !utf8.ValidString(got) {
			t.Errorf("appendCapped(%q, %q, %d) = %q, %v; want %q, %v", tc.output, tc.part, tc.limit, got, full, tc.want, tc.full)
		}
	}
}
