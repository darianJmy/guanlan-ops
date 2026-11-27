package config

import "testing"

func TestGuanlanConfigurationSupportsExternalHostnameAndIndependentBind(t *testing.T) {
	t.Setenv("PORTAL_GUANLAN_URL", "http://backend:8080")
	t.Setenv("SERVICE_PORTAL_INTERNAL_TOKEN", "test-runtime-token")
	t.Setenv("PORTAL_BIND", "0.0.0.0:8080")
	t.Setenv("PORTAL_PUBLIC_URL", "https://portal.example.test:443")
	t.Setenv("PORTAL_MIN_PORT", "18000")
	t.Setenv("PORTAL_MAX_PORT", "18010")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "file:///tmp/socket", "http://user:password@backend:8080", "http://backend:8080/prefix"} {
		t.Setenv("PORTAL_GUANLAN_URL", value)
		if _, err := Load(); err == nil {
			t.Fatalf("accepted 观澜 URL %q", value)
		}
	}
}

func TestLegacyLoginConfigurationCannotReplaceCentralIdentity(t *testing.T) {
	t.Setenv("PORTAL_AUTH_MODE", "standalone")
	t.Setenv("PORTAL_ADMIN_PASSWORD", "obsolete-local-password")
	t.Setenv("PORTAL_OIDC_ISSUER", "https://identity.example.test")
	t.Setenv("PORTAL_GUANLAN_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("legacy login configuration bypassed the required 观澜 identity URL")
	}
}
