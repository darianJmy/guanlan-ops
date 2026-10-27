package textutil

import "testing"

func TestTruncationPolicies(t *testing.T) {
	for _, tc := range []struct {
		value        string
		limit        int
		suffix, want string
	}{
		{"你好世界", 2, "...", "你好..."}, {"你好", 2, "...", "你好"}, {"  abc  ", 3, "", "  a"}, {"abc", 0, "", ""}, {"abc", -1, "", ""}, {"", 0, "...", ""},
	} {
		if got := Truncate(tc.value, tc.limit, tc.suffix); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.value, got, tc.want)
		}
	}
}
