package paging

import "testing"

func TestNormalize(t *testing.T) {
	for _, tc := range []struct {
		index, size         int
		wantIndex, wantSize int
		ok                  bool
	}{
		{0, 0, 1, DefaultSize, true},
		{3, 50, 3, 50, true},
		{MaxIndex, MaxSize, MaxIndex, MaxSize, true},
		{-1, 20, -1, 20, false},
		{1, -1, 1, -1, false},
		{1, MaxSize + 1, 1, MaxSize + 1, false},
		{MaxIndex + 1, 20, MaxIndex + 1, 20, false},
	} {
		index, size := tc.index, tc.size
		if ok := Normalize(&index, &size); ok != tc.ok || index != tc.wantIndex || size != tc.wantSize {
			t.Errorf("Normalize(%d, %d) = %d, %d, %v; want %d, %d, %v", tc.index, tc.size, index, size, ok, tc.wantIndex, tc.wantSize, tc.ok)
		}
	}
}
