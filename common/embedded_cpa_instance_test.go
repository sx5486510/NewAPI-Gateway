package common

import (
	"os"
	"path/filepath"
	"testing"
)

func resetLocalGatewayInstanceIDForTest() {
	localGatewayInstanceIDMu.Lock()
	defer localGatewayInstanceIDMu.Unlock()
	localGatewayInstanceIDCached = ""
}

func TestLocalGatewayInstanceIDPrefersExplicitEnv(t *testing.T) {
	t.Setenv("GATEWAY_INSTANCE_ID", "Prod 37")
	resetLocalGatewayInstanceIDForTest()

	if got := LocalGatewayInstanceID(); got != "prod-37" {
		t.Fatalf("LocalGatewayInstanceID() = %q, want %q", got, "prod-37")
	}
	if got := LocalEmbeddedCPAProviderName(); got != "__embedded_cpa__@prod-37" {
		t.Fatalf("LocalEmbeddedCPAProviderName() = %q", got)
	}
}

func TestLocalGatewayInstanceIDPersistsGeneratedValue(t *testing.T) {
	t.Setenv("GATEWAY_INSTANCE_ID", "")
	t.Setenv("CPA_RUNTIME_DIR", filepath.Join(t.TempDir(), "cpa-runtime"))
	resetLocalGatewayInstanceIDForTest()

	first := LocalGatewayInstanceID()
	if first == "" || first == "default" {
		t.Fatalf("expected generated instance id, got %q", first)
	}

	resetLocalGatewayInstanceIDForTest()
	second := LocalGatewayInstanceID()
	if second != first {
		t.Fatalf("expected persisted instance id %q, got %q", first, second)
	}

	path := persistentGatewayInstanceIDPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted id: %v", err)
	}
	if got := sanitizeGatewayInstanceID(string(data)); got != first {
		t.Fatalf("persisted file = %q, want %q", got, first)
	}
}

func TestEmbeddedCPAProviderHelpersRecognizeScopedNames(t *testing.T) {
	if !IsEmbeddedCPAProviderName("__embedded_cpa__@prod-37") {
		t.Fatal("expected scoped embedded provider name to be recognized")
	}
	if !IsEmbeddedCPAProviderName("__embedded_cpa__") {
		t.Fatal("expected legacy embedded provider name to be recognized")
	}
	if IsEmbeddedCPAProviderName("regular-provider") {
		t.Fatal("regular provider must not be treated as embedded")
	}
}

func TestEmbeddedCPALogLabelUsesLocalInstanceAndProvider(t *testing.T) {
	t.Setenv("GATEWAY_INSTANCE_ID", "Prod 37")
	resetLocalGatewayInstanceIDForTest()

	if got := EmbeddedCPALogLabel(); got != "instance_id=prod-37 provider=__embedded_cpa__@prod-37" {
		t.Fatalf("EmbeddedCPALogLabel() = %q", got)
	}
}
