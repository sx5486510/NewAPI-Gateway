package cpa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir unavailable: %v", err)
	}

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{`~\bar`, filepath.Join(home, "bar")},
		{"/abs/path", "/abs/path"},
		{"C:\\abs\\path", "C:\\abs\\path"},
		{"relative/path", "relative/path"},
	}

	for _, tc := range tests {
		got := expandHome(tc.in)
		if got != tc.want {
			t.Errorf("expandHome(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEnsureAuthDir(t *testing.T) {
	tmp := t.TempDir()

	// Case 1: fresh dir under temp
	authDir := filepath.Join(tmp, "fresh-auth")
	if err := ensureAuthDir(authDir); err != nil {
		t.Fatalf("ensureAuthDir fresh: %v", err)
	}
	if st, err := os.Stat(authDir); err != nil || !st.IsDir() {
		t.Fatalf("auth dir not created: %v", err)
	}

	// Case 2: already exists (idempotent)
	if err := ensureAuthDir(authDir); err != nil {
		t.Fatalf("ensureAuthDir idempotent: %v", err)
	}

	// Case 3: tilde expansion (create under real home)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("UserHomeDir unavailable: %v", err)
	}
	tildeDir := "~/.cpa-test-" + filepath.Base(tmp)
	resolved := filepath.Join(home, ".cpa-test-"+filepath.Base(tmp))
	t.Cleanup(func() { _ = os.RemoveAll(resolved) })

	if err := ensureAuthDir(tildeDir); err != nil {
		t.Fatalf("ensureAuthDir tilde: %v", err)
	}
	if st, err := os.Stat(resolved); err != nil || !st.IsDir() {
		t.Fatalf("tilde-expanded dir not created at %s: %v", resolved, err)
	}

	// Case 4: empty path -> error
	if err := ensureAuthDir(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("ensureAuthDir empty expected error, got %v", err)
	}
}
