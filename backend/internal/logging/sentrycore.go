// Package logging bridges zap into Sentry structured logs.
package logging

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// SentryCore tees zap entries into Sentry structured logs, carrying zap
// fields as log attributes. The Sentry logger is resolved lazily per write,
// so the core can be attached before sentry.Init runs — earlier entries are
// dropped by the SDK's noop logger.
type SentryCore struct {
	zapcore.LevelEnabler
	fields   []zapcore.Field
	newEntry func(zapcore.Level) sentry.LogEntry
}

func NewSentryCore(enab zapcore.LevelEnabler) *SentryCore {
	return &SentryCore{LevelEnabler: enab, newEntry: sentryEntry}
}

func (c *SentryCore) With(fields []zapcore.Field) zapcore.Core {
	clone := &SentryCore{LevelEnabler: c.LevelEnabler, newEntry: c.newEntry}
	clone.fields = append(append([]zapcore.Field{}, c.fields...), fields...)
	return clone
}

func (c *SentryCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c *SentryCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	entry := c.newEntry(ent.Level)

	enc := zapcore.NewMapObjectEncoder()
	for i := range c.fields {
		c.fields[i].AddTo(enc)
	}
	for i := range fields {
		fields[i].AddTo(enc)
	}
	for k, v := range enc.Fields {
		addAttr(entry, k, v)
	}

	if ent.LoggerName != "" {
		entry.String("logger", ent.LoggerName)
	}
	if ent.Caller.Defined {
		entry.String("caller", ent.Caller.TrimmedPath())
	}
	if ent.Stack != "" {
		entry.String("stacktrace", ent.Stack)
	}

	entry.Emit(ent.Message)
	return nil
}

func (c *SentryCore) Sync() error {
	sentry.Flush(2 * time.Second)
	return nil
}

// sentryEntry maps a zap level to a Sentry log entry. Fatal-ish levels use
// LFatal, which records fatal severity without exiting — zap owns process
// teardown for Fatal/Panic.
func sentryEntry(level zapcore.Level) sentry.LogEntry {
	logger := sentry.NewLogger(context.Background())
	switch level {
	case zapcore.DebugLevel:
		return logger.Debug()
	case zapcore.InfoLevel:
		return logger.Info()
	case zapcore.WarnLevel:
		return logger.Warn()
	case zapcore.ErrorLevel:
		return logger.Error()
	default:
		return logger.LFatal()
	}
}

// addAttr converts a MapObjectEncoder value to a typed Sentry attribute.
func addAttr(e sentry.LogEntry, key string, v any) {
	switch t := v.(type) {
	case string:
		e.String(key, t)
	case bool:
		e.Bool(key, t)
	case int:
		e.Int(key, t)
	case int64:
		e.Int64(key, t)
	case uint64:
		if t <= 1<<63-1 {
			e.Int64(key, int64(t))
		} else {
			e.String(key, fmt.Sprintf("%d", t))
		}
	case float64:
		e.Float64(key, t)
	case float32:
		e.Float64(key, float64(t))
	case time.Duration:
		e.String(key, t.String())
	case time.Time:
		e.String(key, t.Format(time.RFC3339Nano))
	case error:
		e.String(key, t.Error())
	default:
		e.String(key, fmt.Sprintf("%+v", t))
	}
}
