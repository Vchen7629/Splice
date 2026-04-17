package test

import (
	"net/http"
	"net/http/httptest"
	"shared/middleware"
	"testing"
)

func NewCorsServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(middleware.Cors(mux))
}
