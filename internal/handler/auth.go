package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authMiddleware requires a shared token for every request when token is non-empty.
// Accepted sources (first match wins):
//  1. Authorization: Bearer <token>
//  2. X-API-Token: <token>
//  3. Query ?token=<token>  (needed for EventSource / SSE)
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	want := s.token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := tokenFromRequest(r)
		if !tokenEqual(want, got) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="fm350-manager"`)
			http.Error(w, "unauthorized: set Authorization Bearer, X-API-Token, or ?token=", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		const p = "Bearer "
		if strings.HasPrefix(h, p) {
			return strings.TrimSpace(h[len(p):])
		}
		if strings.HasPrefix(strings.ToLower(h), "bearer ") {
			return strings.TrimSpace(h[7:])
		}
	}
	if t := r.Header.Get("X-API-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func tokenEqual(want, got string) bool {
	if want == "" {
		return true
	}
	// subtle.ConstantTimeCompare requires equal length; mismatch is always false.
	if len(want) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}
