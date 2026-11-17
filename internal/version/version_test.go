package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.1", "v0.1.1", 0},
		{"0.1.1", "v0.1.1", 0},
		{"v0.1.1.1", "v0.1.1", 1},
		{"v0.1.1", "v0.1.1.1", -1},
		{"v0.1.1.1", "0.1.1.1", 0},
		{"v0.1.2", "v0.1.1.1", 1},
		{"v0.2.0", "v0.1.9", 1},
		{"v1.0.0", "v0.9.99", 1},
		{"v0.1.0", "v0.1.1", -1},
		{"v0.1.2", "v0.1.2-rc1", 1},
	}
	for _, tc := range cases {
		if got := Compare(tc.a, tc.b); got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
