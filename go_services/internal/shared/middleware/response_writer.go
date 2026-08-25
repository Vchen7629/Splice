package middleware

import "net/http"

// wrapper to extend http response writer to expose
// the status codes
type wrappedWriter struct {
	http.ResponseWriter
	StatusCode int
}

func (w *wrappedWriter) WriteHeader(statuscode int) {
	w.ResponseWriter.WriteHeader(statuscode)
	w.StatusCode = statuscode
}

// lets http.ResponseController to use underlying ResponseWriter
// Needed for streaming handles (Like SSE) downstream of this middleware
func (w *wrappedWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
