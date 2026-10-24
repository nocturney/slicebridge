package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// These tests reserve permits only; no MakerWorld HTTP, browser protocol,
// frontend event or physical printer operation occurs.
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
