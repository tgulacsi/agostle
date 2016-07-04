package converter

import "testing"

func TestTagIndex(t *testing.T) {
	for i, tc := range []struct {
		txt  string
		a, b int
	}{
		{"<html><head><meta http-equiv=\"content-type\" content=\"text/html; charset=us-ascii\"></head><body dir=\"auto\"><blockquote type=\"cite\"><div></div></blockquote></body></html>", 89, 106},
	} {
		a, b := tagIndex([]byte(tc.txt), "body")
		if !(a == tc.a && b == tc.b) {
			t.Errorf("%d: got (%d,%d), wanted (%d,%d)", i, a, b, tc.a, tc.b)
		}
	}
}
