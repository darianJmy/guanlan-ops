package httpclient

import "net/http"

type Auth struct{ Type, Username, Password, Token string }

func (auth Auth) Apply(req *http.Request) {
	switch auth.Type {
	case "basic":
		req.SetBasicAuth(auth.Username, auth.Password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.Token)
	}
}

// Transport clones each request before applying headers and authentication.
type Transport struct {
	Base    http.RoundTripper
	Auth    Auth
	Headers map[string]string
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	if cloned.Header == nil {
		cloned.Header = make(http.Header)
	}
	for key, value := range t.Headers {
		cloned.Header.Set(key, value)
	}
	t.Auth.Apply(cloned)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}
