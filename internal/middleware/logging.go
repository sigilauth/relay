package middleware

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// hijackableResponseWriter wraps chi's WrapResponseWriter and implements http.Hijacker
type hijackableResponseWriter struct {
	middleware.WrapResponseWriter
}

// Hijack implements http.Hijacker by forwarding to the underlying ResponseWriter
func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := h.Unwrap().(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hijacker.Hijack()
}

func StructuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap with chi's response writer for status/bytes tracking
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			// Add Hijacker support for WebSocket upgrade
			hww := &hijackableResponseWriter{WrapResponseWriter: ww}

			defer func() {
				duration := time.Since(start)

				route := r.URL.Path
				if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil && routeCtx.RoutePattern() != "" {
					route = routeCtx.RoutePattern()
				}

				requestID := middleware.GetReqID(r.Context())

				attrs := []any{
					slog.String("method", r.Method),
					slog.String("path", route),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
					slog.String("remote_addr", r.RemoteAddr),
				}

				if requestID != "" {
					attrs = append(attrs, slog.String("request_id", requestID))
				}

				if ww.Status() >= 500 {
					logger.Error("HTTP request", attrs...)
				} else if ww.Status() >= 400 {
					logger.Warn("HTTP request", attrs...)
				} else {
					logger.Info("HTTP request", attrs...)
				}
			}()

			next.ServeHTTP(hww, r)
		})
	}
}
