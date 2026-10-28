package service

import (
	"NewAPI-Gateway/model"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestFetchOpenAIModelsDoesNotDuplicateV1Path(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	defer server.Close()

	models, err := fetchOpenAIModels(server.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("fetchOpenAIModels() error = %v", err)
	}
	if requestedPath != "/v1/models" {
		t.Fatalf("requested path = %q, want %q", requestedPath, "/v1/models")
	}
	if want := []string{"test-model"}; !reflect.DeepEqual(models, want) {
		t.Fatalf("models = %v, want %v", models, want)
	}
}

func TestCollectRequiredTokenGroups(t *testing.T) {
	pricingList := []*model.ModelPricing{
		{EnableGroups: `["default", "vip"]`},
		{EnableGroups: `["beta", " "]`},
		{EnableGroups: `["vip"]`},
		{EnableGroups: `invalid json`},
	}

	got := collectRequiredTokenGroups(pricingList)
	want := []string{"beta", "default", "vip"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectRequiredTokenGroups() = %v, want %v", got, want)
	}
}

func TestGetMissingTokenGroups(t *testing.T) {
	requiredGroups := []string{"beta", "default", "vip"}
	tokens := []UpstreamToken{
		{Group: ""},
		{Group: " vip "},
	}

	got := getMissingTokenGroups(requiredGroups, tokens)
	want := []string{"beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getMissingTokenGroups() = %v, want %v", got, want)
	}
}

func TestProviderRebuildLockSerializesSameProvider(t *testing.T) {
	lock := providerRebuildLock(987654)
	lock.Lock()
	entered := make(chan struct{})
	released := make(chan struct{})
	go func() {
		lock.Lock()
		close(entered)
		lock.Unlock()
		close(released)
	}()
	select {
	case <-entered:
		t.Fatal("second rebuild acquired the provider lock too early")
	case <-time.After(20 * time.Millisecond):
	}
	lock.Unlock()
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("second rebuild did not acquire the provider lock after release")
	}
}
