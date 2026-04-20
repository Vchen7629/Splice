//go:build integration

package middleware_test

import (
	"net/http"
	"shared/test"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCorsIntegration(t *testing.T) {
	ts := test.NewCorsServer(t)
	defer ts.Close()

	tests := []struct {
		name              string
		method            string
		origin            string
		expectStatus      int
		expectAllowOrigin string
	}{
		{
			name:              "Allowed origin GET receives CORS headers",
			method:            http.MethodGet,
			origin:            "http://localhost:5173",
			expectStatus:      http.StatusOK,
			expectAllowOrigin: "http://localhost:5173",
		},
		{
			name:         "Disallowed origin GET receives no CORS headers",
			method:       http.MethodGet,
			origin:       "http://evil.com",
			expectStatus: http.StatusOK,
		},
		{
			name:              "Allowed origin OPTIONS preflight returns 204",
			method:            http.MethodOptions,
			origin:            "http://localhost:5173",
			expectStatus:      http.StatusNoContent,
			expectAllowOrigin: "http://localhost:5173",
		},
		{
			name:         "Disallowed origin OPTIONS preflight returns 403",
			method:       http.MethodOptions,
			origin:       "http://evil.com",
			expectStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, ts.URL+"/", nil)
			require.NoError(t, err)
			req.Header.Set("Origin", tt.origin)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.expectStatus, resp.StatusCode)
			assert.Equal(t, tt.expectAllowOrigin, resp.Header.Get("Access-Control-Allow-Origin"))
		})
	}
}
