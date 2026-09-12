package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOptimizerHTTPRejectsCrossOriginAndArbitraryActions(t *testing.T) {
	a := &optimizerApp{token: "secret"}
	for _, tc := range []struct {
		path, host, origin, token string
		code                      int
	}{
		{"/api/apply", "127.0.0.1:8765", "https://evil.example", "secret", 403},
		{"/api/restore", "127.0.0.1:8765", "http://127.0.0.1:8765", "wrong", 403},
		{"/api/apply", "evil.example:8765", "http://evil.example:8765", "secret", 403},
		{"/api/run-shell", "127.0.0.1:8765", "http://127.0.0.1:8765", "secret", 404},
	} {
		request := httptest.NewRequest("POST", "http://"+tc.host+tc.path, strings.NewReader(url.Values{"token": {tc.token}}.Encode()))
		request.Header.Set("Origin", tc.origin)
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		a.ServeHTTP(w, request)
		if w.Code != tc.code {
			t.Fatal(tc.path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "optimizer-bootstrap") {
		t.Fatal("local optimizer unavailable")
	}
}
