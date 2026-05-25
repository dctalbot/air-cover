package api

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestTrustForwardedHeadersMarksConfiguredProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:12345"

	var trusted bool
	handler := TrustForwardedHeaders([]netip.Prefix{netip.MustParsePrefix("::1/128")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted = forwardedHeadersTrusted(r)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !trusted {
		t.Fatal("expected forwarded headers to be trusted for configured proxy")
	}
}

func TestTrustForwardedHeadersIgnoresUnconfiguredProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:12345"

	var trusted bool
	handler := TrustForwardedHeaders([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted = forwardedHeadersTrusted(r)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if trusted {
		t.Fatal("expected forwarded headers not to be trusted for unconfigured proxy")
	}
}

func TestTrustForwardedHeadersIgnoresMalformedRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-an-ip"

	var trusted bool
	handler := TrustForwardedHeaders([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted = forwardedHeadersTrusted(r)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if trusted {
		t.Fatal("expected forwarded headers not to be trusted for malformed remote address")
	}
}

func TestTrustForwardedHeadersWithoutConfiguredProxies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	var trusted bool
	handler := TrustForwardedHeaders(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		trusted = forwardedHeadersTrusted(r)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if trusted {
		t.Fatal("expected forwarded headers not to be trusted without configured proxies")
	}
}

func TestTrustedForwardedProtoNormalizesFirstHeaderValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", " HTTPS, http")

	var got string
	handler := TrustForwardedHeaders([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = trustedForwardedProto(r)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got != "https" {
		t.Fatalf("expected normalized forwarded proto https, got %q", got)
	}
}
