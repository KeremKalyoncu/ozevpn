package server

import "testing"

func TestCompareSemVer(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v6.1.2", "v6.1.2", 0},
		{"v6.1.2", "v6.2.0", -1},
		{"v6.2.0", "v6.1.9", 1},
		{"6.1.2", "v6.1.10", -1},
		{"v6.1.2-rc1", "v6.1.2", 0},
	}
	for _, tc := range cases {
		if got := compareSemVer(tc.a, tc.b); got != tc.want {
			t.Fatalf("compareSemVer(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
