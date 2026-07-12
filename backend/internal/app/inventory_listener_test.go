package app

import (
	"context"
	"testing"
	"time"

	"backend/internal/config"
	"backend/internal/inventory"

	"go.uber.org/zap"
)

func TestStartInventoryListenerShutdown(t *testing.T) {
	lc := &captureLifecycle{}
	// No DSN configured: the run loop fast-fails to connect and backs off. The
	// point is that OnStop cancels and the goroutine exits promptly (no leak).
	StartInventoryListener(lc, config.Config{}, inventory.NewHub(), zap.NewNop())

	if lc.hook.OnStart == nil || lc.hook.OnStop == nil {
		t.Fatal("StartInventoryListener did not register start/stop hooks")
	}
	if err := lc.hook.OnStart(context.Background()); err != nil {
		t.Fatalf("OnStart returned error: %v", err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- lc.hook.OnStop(context.Background()) }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("OnStop returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnStop did not return promptly (goroutine leak)")
	}
}
