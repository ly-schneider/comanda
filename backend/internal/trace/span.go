package trace

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"
)

func StartSpan(ctx context.Context, op, description string) (context.Context, func()) {
	if sentry.SpanFromContext(ctx) == nil {
		return ctx, func() {}
	}
	// sentry.StartSpan derives the returned span's context from ctx. Do NOT
	// use parent.StartChild here: its context derives from the ctx captured
	// when the parent span started (pre-middleware), silently dropping
	// request-scoped values — the enttenant guard then fails closed on every
	// eager-load query.
	span := sentry.StartSpan(ctx, op, sentry.WithDescription(description))
	return span.Context(), span.Finish
}

func IdentifyUser(ctx context.Context, id, email, name string) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	hub.Scope().SetUser(sentry.User{
		ID:       id,
		Email:    email,
		Username: name,
	})
}

func Tag(ctx context.Context, key, value string) {
	span := sentry.SpanFromContext(ctx)
	if span == nil {
		return
	}
	span.SetTag(key, value)
}

func Data(ctx context.Context, key string, value any) {
	span := sentry.SpanFromContext(ctx)
	if span == nil {
		return
	}
	span.SetData(key, value)
}

func Breadcrumb(ctx context.Context, category, message string, data map[string]any) {
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		return
	}
	hub.AddBreadcrumb(&sentry.Breadcrumb{
		Category:  category,
		Message:   message,
		Level:     sentry.LevelInfo,
		Data:      data,
		Timestamp: time.Now(),
	}, nil)
}

func Err(ctx context.Context, err error) {
	if err == nil {
		return
	}
	Tag(ctx, "error", "true")
	Data(ctx, "error.message", err.Error())
}
