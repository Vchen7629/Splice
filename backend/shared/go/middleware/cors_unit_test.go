//go:build unit

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"shared/middleware"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCors(t *testing.T) {
	tests := []struct {
		name              string
		method            string
		origin            string
		expectStatus      int
		expectAllowOrigin string
		expectNextCalled  bool
	}{
		{
			name:              "Allowed origin GET receives CORS headers",
			method:            http.MethodGet,
			origin:            "http://localhost:5173",
			expectStatus:      http.StatusOK,
			expectAllowOrigin: "http://localhost:5173",
			expectNextCalled:  true,
		},
		{
			name:             "Disallowed origin GET receives no CORS headers",
			method:           http.MethodGet,
			origin:           "http://evil.com",
			expectStatus:     http.StatusOK,
			expectNextCalled: true,
		},
		{
			name:              "Allowed origin OPTIONS returns 204, next not called",
			method:            http.MethodOptions,
			origin:            "http://localhost:5173",
			expectStatus:      http.StatusNoContent,
			expectAllowOrigin: "http://localhost:5173",
			expectNextCalled:  false,
		},
		{
			name:             "Disallowed origin OPTIONS returns 403, next not called",
			method:           http.MethodOptions,
			origin:           "http://evil.com",
			expectStatus:     http.StatusForbidden,
			expectNextCalled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled := false
			mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(tt.method, "/test", nil)
			req.Header.Set("Origin", tt.origin)
			recorder := httptest.NewRecorder()

			middleware.Cors(mockHandler).ServeHTTP(recorder, req)

			assert.Equal(t, tt.expectStatus, recorder.Code)
			assert.Equal(t, tt.expectAllowOrigin, recorder.Header().Get("Access-Control-Allow-Origin"))
			assert.Equal(t, tt.expectNextCalled, handlerCalled)
		})
	}
}
