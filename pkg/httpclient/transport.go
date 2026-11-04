package httpclient

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"net/http"
)

type Auth struct{ Type, Username, Password, Token string }

func (auth Auth) Apply(req *http.Request) {
	switch auth.Type {
	case "basic":
		req.SetBasicAuth(auth.Username, auth.Password)
	case "api_key":
		req.Header.Set("Authorization", "ApiKey "+auth.Token)
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
	ctx, span := otel.Tracer("guanlan/httpclient").Start(cloned.Context(), cloned.Method+" "+cloned.URL.Hostname(), trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	cloned = cloned.WithContext(ctx)
	span.SetAttributes(attribute.String("http.request.method", cloned.Method), attribute.String("server.address", cloned.URL.Hostname()))
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(cloned.Header))
	response, err := base.RoundTrip(cloned)
	if err != nil {
		span.SetStatus(codes.Error, "HTTP transport error")
	} else {
		span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
		if response.StatusCode >= 400 {
			span.SetStatus(codes.Error, "HTTP error")
		}
	}
	return response, err
}
