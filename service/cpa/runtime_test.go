package cpa

import (
	"sync"
	"testing"
)

type testLifecycleHooks struct{}

func (h *testLifecycleHooks) OnCPAReady(baseURL, apiKey string)      {}
func (h *testLifecycleHooks) OnCPAUnavailable()                      {}
func (h *testLifecycleHooks) ScheduleCPASync()                       {}

func TestRuntimeFieldsNonNil(t *testing.T) {
	hooks := &testLifecycleHooks{}
	runtime, err := NewRuntime(t.TempDir(), hooks)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if runtime.Store == nil {
		t.Fatal("Store is nil")
	}
	if runtime.Manager == nil {
		t.Fatal("Manager is nil")
	}
	if runtime.Proxy == nil {
		t.Fatal("Proxy is nil")
	}
	if runtime.OAuth == nil {
		t.Fatal("OAuth is nil")
	}
	if runtime.Panel == nil {
		t.Fatal("Panel is nil")
	}
}

func TestSetDefaultRuntimeAtomicReplace(t *testing.T) {
	hooks := &testLifecycleHooks{}
	rt1, err := NewRuntime(t.TempDir(), hooks)
	if err != nil {
		t.Fatalf("NewRuntime 1: %v", err)
	}
	rt2, err := NewRuntime(t.TempDir(), hooks)
	if err != nil {
		t.Fatalf("NewRuntime 2: %v", err)
	}

	SetDefaultRuntime(rt1)
	if got := DefaultRuntime(); got != rt1 {
		t.Fatal("first set did not take effect")
	}

	SetDefaultRuntime(rt2)
	if got := DefaultRuntime(); got != rt2 {
		t.Fatal("second set did not replace")
	}
}

func TestDefaultRuntimeConcurrentReadSafe(t *testing.T) {
	hooks := &testLifecycleHooks{}
	rt, err := NewRuntime(t.TempDir(), hooks)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	SetDefaultRuntime(rt)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if got := DefaultRuntime(); got != rt {
					t.Error("concurrent read returned wrong runtime")
				}
			}
		}()
	}
	wg.Wait()
}
