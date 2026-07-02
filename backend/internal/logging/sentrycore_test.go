package logging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type fakeEntry struct {
	attrs   map[string]any
	emitted []string
}

func newFakeEntry() *fakeEntry {
	return &fakeEntry{attrs: map[string]any{}}
}

func (f *fakeEntry) WithCtx(context.Context) sentry.LogEntry { return f }
func (f *fakeEntry) String(k, v string) sentry.LogEntry      { f.attrs[k] = v; return f }
func (f *fakeEntry) StringSlice(k string, v []string) sentry.LogEntry {
	f.attrs[k] = v
	return f
}
func (f *fakeEntry) Int(k string, v int) sentry.LogEntry     { f.attrs[k] = v; return f }
func (f *fakeEntry) Int64(k string, v int64) sentry.LogEntry { f.attrs[k] = v; return f }
func (f *fakeEntry) Int64Slice(k string, v []int64) sentry.LogEntry {
	f.attrs[k] = v
	return f
}
func (f *fakeEntry) Float64(k string, v float64) sentry.LogEntry { f.attrs[k] = v; return f }
func (f *fakeEntry) Float64Slice(k string, v []float64) sentry.LogEntry {
	f.attrs[k] = v
	return f
}
func (f *fakeEntry) Bool(k string, v bool) sentry.LogEntry { f.attrs[k] = v; return f }
func (f *fakeEntry) BoolSlice(k string, v []bool) sentry.LogEntry {
	f.attrs[k] = v
	return f
}
func (f *fakeEntry) Emit(args ...interface{}) {
	for _, a := range args {
		f.emitted = append(f.emitted, a.(string))
	}
}
func (f *fakeEntry) Emitf(format string, _ ...interface{}) {
	f.emitted = append(f.emitted, format)
}

func newTestCore(enab zapcore.LevelEnabler) (*SentryCore, map[zapcore.Level]*fakeEntry) {
	entries := map[zapcore.Level]*fakeEntry{}
	core := NewSentryCore(enab)
	core.newEntry = func(level zapcore.Level) sentry.LogEntry {
		e := newFakeEntry()
		entries[level] = e
		return e
	}
	return core, entries
}

func TestSentryCoreForwardsWarnAndAboveWithFields(t *testing.T) {
	core, entries := newTestCore(zapcore.WarnLevel)
	logger := zap.New(core)

	logger.Info("below threshold")
	logger.Warn("Plunk not configured, skipping OTP email",
		zap.String("to", "user@example.com"),
	)
	logger.Error("failed to send OTP email",
		zap.Error(errors.New("boom")),
		zap.Int("status", 502),
		zap.Float64("latency_ms", 12.5),
		zap.Bool("retried", true),
		zap.Duration("elapsed", 3*time.Second),
	)

	if _, ok := entries[zapcore.InfoLevel]; ok {
		t.Fatal("info entry should not be forwarded")
	}

	warn := entries[zapcore.WarnLevel]
	if warn == nil || len(warn.emitted) != 1 || warn.emitted[0] != "Plunk not configured, skipping OTP email" {
		t.Fatalf("unexpected warn emission: %+v", warn)
	}
	if warn.attrs["to"] != "user@example.com" {
		t.Fatalf("warn attrs = %+v", warn.attrs)
	}

	errEntry := entries[zapcore.ErrorLevel]
	if errEntry == nil || errEntry.emitted[0] != "failed to send OTP email" {
		t.Fatalf("unexpected error emission: %+v", errEntry)
	}
	if errEntry.attrs["error"] != "boom" {
		t.Fatalf("error attr = %v", errEntry.attrs["error"])
	}
	if errEntry.attrs["status"] != int64(502) {
		t.Fatalf("status attr = %v (%T)", errEntry.attrs["status"], errEntry.attrs["status"])
	}
	if errEntry.attrs["latency_ms"] != 12.5 {
		t.Fatalf("latency attr = %v", errEntry.attrs["latency_ms"])
	}
	if errEntry.attrs["retried"] != true {
		t.Fatalf("retried attr = %v", errEntry.attrs["retried"])
	}
	if errEntry.attrs["elapsed"] != "3s" {
		t.Fatalf("elapsed attr = %v", errEntry.attrs["elapsed"])
	}
}

func TestSentryCoreWithFieldsAccumulate(t *testing.T) {
	core, entries := newTestCore(zapcore.WarnLevel)
	logger := zap.New(core).With(zap.String("component", "email"))

	logger.Warn("skipped", zap.String("reason", "no creds"))

	warn := entries[zapcore.WarnLevel]
	if warn.attrs["component"] != "email" || warn.attrs["reason"] != "no creds" {
		t.Fatalf("attrs = %+v", warn.attrs)
	}
}
