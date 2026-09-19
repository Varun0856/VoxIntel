package main

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

func basicAuth(next http.Handler) http.Handler {
	credentials := parseAuthUsers(os.Getenv("AUTH_USERS"))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if len(credentials) == 0 {
			http.Error(w, "server misconfigured: no AUTH_USERS set", http.StatusInternalServerError)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok || !validCredentials(credentials, user, pass) {
			w.Header().Set("WWW-Authenticate", `Basic realm="VoxIntel"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAuthUsers(raw string) map[string]string {
	creds := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 && parts[0] != "" {
			creds[parts[0]] = parts[1]
		}
	}
	return creds
}

func validCredentials(creds map[string]string, user, pass string) bool {
	expected, ok := creds[user]
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(pass)) == 1
}
