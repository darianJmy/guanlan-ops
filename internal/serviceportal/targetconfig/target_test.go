package targetconfig

import "testing"

func TestHTTPURLKeepsInitialPageSeparateFromOrigin(t *testing.T) {
	for _, test := range []struct{ raw, path string }{
		{"http://localhost:8848/nacos/?view=all#console", "/nacos/?view=all#console"},
		{"https://example.com", "/"},
		{"https://[2001:db8::1]:8443/a%2Fb?name=a%20b#tab%20one", "/a%2Fb?name=a%20b#tab%20one"},
		{"https://example.com./?", "/?"},
	} {
		u, err := ParseHTTPURL(test.raw)
		if err != nil {
			t.Fatalf("valid URL %q: %v", test.raw, err)
		}
		if got := OpenPath(u); got != test.path {
			t.Errorf("initial page %q: got %q, want %q", test.raw, got, test.path)
		}
	}
}

func TestHTTPURLRejectsUnsafeOrUnusableAddresses(t *testing.T) {
	for _, raw := range []string{
		"", "example.com", "ftp://example.com/", "javascript:alert(1)", "http:example.com", "https:///path",
		"https://user:password@example.com/", "http://example.com:0/", "http://example.com:65536/", "http://example.com:/", "http://example.com:abc/",
		"http://[example.com]/", "http://-example.com/", "http://example..com/", "http://example.com/white space", "http://example.com/\nheader", "http://example.com\\evil",
		"http://example.com//evil", "http://example.com/%2f/evil", "http://example.com/%5cpath", "http://example.com/_portal/access?ticket=unexpected", "http://example.com/%5fportal/access",
	} {
		if _, err := ParseHTTPURL(raw); err == nil {
			t.Errorf("invalid URL accepted: %q", raw)
		}
	}
}

func TestLegacyResourcesKeepKubernetesAccess(t *testing.T) {
	if got, err := Type(""); err != nil || got != Kubernetes {
		t.Fatalf("legacy access type = %q, %v", got, err)
	}
	if _, err := Type("unknown"); err == nil {
		t.Fatal("unknown access type accepted")
	}
}
