package middleware

import (
	"backend/internal/trace"
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	chiMw "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func TraceRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if span := sentry.SpanFromContext(r.Context()); span != nil {
			if rctx := chi.RouteContext(r.Context()); rctx != nil {
				if pattern := rctx.RoutePattern(); pattern != "" {
					span.Name = r.Method + " " + pattern
					span.SetTag("http.route", pattern)
				}
			}
		}
	})
}

func LogHTTPErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		latencyMs := float64(time.Since(start).Microseconds()) / 1000.0

		// Group by the route pattern, not the raw path — otherwise every order id
		// in /v1/orders/{orderId}/payment spawns its own Sentry issue.
		route := r.URL.Path
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				route = pattern
			}
		}

		reqID := chiMw.GetReqID(r.Context())
		fields := []zap.Field{
			zap.Int("status", rec.status),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("route", route),
			zap.Float64("latency_ms", latencyMs),
			zap.String("remote_ip", r.RemoteAddr),
			zap.String("request_id", reqID),
		}

		// Warn/error entries reach Sentry logs through the zap tee core.
		switch {
		case rec.status >= 500:
			trace.Tag(r.Context(), "http.status_code", fmt.Sprintf("%d", rec.status))
			trace.Tag(r.Context(), "error", "true")
			zap.L().Error(fmt.Sprintf("HTTP %d: %s %s", rec.status, r.Method, r.URL.Path), fields...)

			if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
				hub.WithScope(func(scope *sentry.Scope) {
					scope.SetTag("status", fmt.Sprintf("%d", rec.status))
					scope.SetTag("method", r.Method)
					scope.SetTag("http.route", route)
					scope.SetTag("path", r.URL.Path)
					hub.CaptureMessage(fmt.Sprintf("HTTP %d: %s %s", rec.status, r.Method, route))
				})
			}
		case rec.status >= 400:
			trace.Tag(r.Context(), "http.status_code", fmt.Sprintf("%d", rec.status))
			zap.L().Warn(fmt.Sprintf("HTTP %d: %s %s", rec.status, r.Method, r.URL.Path), fields...)
		default:
			zap.L().Info("http request", fields...)
		}
	})
}
