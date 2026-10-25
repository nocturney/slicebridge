package main

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

// Tests exercise permits and deferred external-app protocol handling with
// isolated temp settings and a no-network launch hook. No MakerWorld HTTP,
// OS protocol registration, frontend event or physical printer operation.
func TestMakerWorldConcurrentImportReservationsAreBounded(t *testing.T) {
	b := &BrowserBridge{ctx: context.Background()}
	const attempts = 100
	var wg sync.WaitGroup
	var start = make(chan struct{})
	var accepted atomic.Int64
	var busy atomic.Int64
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			raw := fmt.Sprintf("slicebridge://import?url=https%%3A%%2F%%2Fmakerworld.com%%2Fmodel.3mf%%3Fid%%3D%d", i)
			ctx, ok, atCapacity := b.reserveActiveImport(raw)
			if ok {
				if ctx == nil || atCapacity {
					t.Errorf("accepted with no context or capacity false: %d", i)
				}
				accepted.Add(1)
			} else if atCapacity {
				busy.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if got := int(accepted.Load()); got != maxConcurrentMakerWorldImports {
		t.Fatalf("accepted %d concurrent imports, want exactly %d", got, maxConcurrentMakerWorldImports)
	}
	if got := int(busy.Load()); got != attempts-maxConcurrentMakerWorldImports {
		t.Fatalf("denied %d at capacity, want %d", got, attempts-maxConcurrentMakerWorldImports)
	}
	b.mu.Lock()
	n := len(b.active)
	keys := make([]string, 0, n)
	for key := range b.active {
		keys = append(keys, key)
	}
	b.mu.Unlock()
	if n != maxConcurrentMakerWorldImports {
		t.Fatalf("actual active set %d exceeded limit", n)
	}
	for _, key := range keys {
		b.releaseActiveImport(key)
	}
	b.mu.Lock()
	remaining := len(b.active)
	b.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("permit leak: %d imports still active", remaining)
	}
}

func TestMakerWorldImportDuplicateAndCapacityRecovery(t *testing.T) {
	b := &BrowserBridge{ctx: context.Background()}
	keys := make([]string, 0, maxConcurrentMakerWorldImports)
	for i := 0; i < maxConcurrentMakerWorldImports; i++ {
		key := fmt.Sprintf("safe-url-%d", i)
		keys = append(keys, key)
		_, ok, full := b.reserveActiveImport(key)
		if !ok || full {
			t.Fatalf("failed to reserve available slot %d", i)
		}
	}
	_, ok, full := b.reserveActiveImport(keys[0])
	if ok || full {
		t.Fatal("duplicate must be ignored rather than reported as full")
	}
	_, ok, full = b.reserveActiveImport("next-link")
	if ok || !full {
		t.Fatal("excess distinct URL must be rejected as capacity exceeded")
	}
	b.releaseActiveImport(keys[0])
	ctx, ok, full := b.reserveActiveImport("next-link")
	if !ok || full || ctx == nil {
		t.Fatal("released slot must be reusable")
	}
	for _, key := range keys[1:] {
		b.releaseActiveImport(key)
	}
	b.releaseActiveImport("next-link")
	// Extra release must be harmless rather than underflow a counter.
	b.releaseActiveImport("next-link")
}

// A desktop may receive many URL-open events before frontend startup.
// They must stay queued and resume as permits are released, not emit busy
// events before the frontend listener can receive them.
func TestMakerWorldStartupQueueDrainsUnderConcurrencyLimit(t *testing.T) {
	developerModeFixture(t)
	if enabled, err := (&App{}).SetDeveloperMode(true); err != nil || !enabled {
		t.Fatalf("enable isolated test Developer Mode: %v, %v", enabled, err)
	}
	queued := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		queued = append(queued, fmt.Sprintf("early-%02d", i))
	}
	b := &BrowserBridge{waiting: append([]string(nil), queued...)}
	started := make([]string, 0, len(queued))
	b.launchForTest = func(ctx context.Context, raw string) {
		if ctx == nil {
			t.Fatal("queued import lost native context")
		}
		started = append(started, raw)
	}
	b.setContext(context.Background())
	if len(started) != 0 {
		t.Fatal("startup import launched before frontend subscriptions were ready")
	}
	b.markFrontendReady()
	if len(started) != maxConcurrentMakerWorldImports {
		t.Fatalf("startup launched %d, limit is %d", len(started), maxConcurrentMakerWorldImports)
	}
	for i := 0; i < len(queued)-maxConcurrentMakerWorldImports; i++ {
		b.releaseActiveImport(started[i])
		if got := len(started); got != maxConcurrentMakerWorldImports+i+1 {
			t.Fatalf("queued import %d silently disappeared: %v", i, started)
		}
	}
	for _, raw := range started[len(started)-maxConcurrentMakerWorldImports:] {
		b.releaseActiveImport(raw)
	}
	if len(started) != len(queued) {
		t.Fatalf("startup lost links: %v", started)
	}
	for i, raw := range queued {
		if started[i] != raw {
			t.Fatalf("queue reordering/loss: got %v expected %v", started, queued)
		}
	}
	b.mu.Lock()
	remaining, active := len(b.waiting), len(b.active)
	b.mu.Unlock()
	if remaining != 0 || active != 0 {
		t.Fatalf("queued imports leaked: waiting=%d active=%d", remaining, active)
	}
}

func TestMakerWorldURLHandlerWaitsForFrontendListeners(t *testing.T) {
	developerModeFixture(t)
	if enabled, err := (&App{}).SetDeveloperMode(true); err != nil || !enabled {
		t.Fatalf("enable isolated test Developer Mode: %v, %v", enabled, err)
	}
	b := &BrowserBridge{}
	first := "slicebridge://import?url=" + url.QueryEscape("https://makerworld.com/api/model.3mf?model=1")
	second := "slicebridge://import?url=" + url.QueryEscape("https://makerworld.com/api/model.3mf?model=2")
	var launched []string
	b.launchForTest = func(ctx context.Context, raw string) {
		if ctx == nil {
			t.Fatal("startup context missing")
		}
		launched = append(launched, raw)
	}
	// URI received even before native Wails OnStartup exists.
	b.acceptArg(first)
	b.setContext(context.Background())
	// Second-instance and macOS OnUrlOpen callbacks may arrive before JS.
	b.acceptArg(second)
	b.acceptArg(first) // A duplicate early link should not waste a queue slot.
	if len(launched) != 0 || len(b.waiting) != 2 {
		t.Fatalf("early URIs processed or lost before frontend readiness: queued=%d launched=%v", len(b.waiting), launched)
	}
	b.markFrontendReady()
	if len(launched) != 2 || launched[0] != first || launched[1] != second {
		t.Fatalf("early URI ordering/mapping lost: %v", launched)
	}
	b.releaseActiveImport(first)
	b.releaseActiveImport(second)
	if len(b.active) != 0 || len(b.waiting) != 0 {
		t.Fatal("early URL permits leaked")
	}
}

func TestMakerWorldImportBeforeAppStartupDoesNotReserve(t *testing.T) {
	b := &BrowserBridge{}
	ctx, ok, full := b.reserveActiveImport("safe-url")
	if ctx != nil || ok || full {
		t.Fatal("no native app context must not launch an import")
	}
	b.mu.Lock()
	n := len(b.active)
	b.mu.Unlock()
	if n != 0 {
		t.Fatal("created a permit without desktop startup")
	}
}
