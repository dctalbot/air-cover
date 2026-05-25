package api

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const forwardedProtoHeader = "X-Forwarded-Proto"

type trustedProxyContextKey struct{}

func TrustForwardedHeaders(proxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(proxies) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if remoteAddrTrusted(r.RemoteAddr, proxies) {
				ctx := context.WithValue(r.Context(), trustedProxyContextKey{}, true)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func remoteAddrTrusted(remoteAddr string, proxies []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}

	for _, proxy := range proxies {
		if proxy.Contains(addr) {
			return true
		}
	}
	return false
}

func forwardedHeadersTrusted(r *http.Request) bool {
	trusted, _ := r.Context().Value(trustedProxyContextKey{}).(bool)
	return trusted
}

func trustedForwardedProto(r *http.Request) string {
	if !forwardedHeadersTrusted(r) {
		return ""
	}

	proto := r.Header.Get(forwardedProtoHeader)
	if idx := strings.IndexByte(proto, ','); idx >= 0 {
		proto = proto[:idx]
	}
	return strings.ToLower(strings.TrimSpace(proto))
}
