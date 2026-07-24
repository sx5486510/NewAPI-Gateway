package cpa

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	body := []byte("{\n  \"access_token\": \"new\"\n}\n")
	if err := writeFileAtomic(path, body, 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("content = %q, want %q", got, body)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("leftover files: %v", names)
	}
}

func TestWriteFileAtomicLeavesOriginalWhenBackupRenameFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	atomicRename = func(string, string) error { return errors.New("rename failed") }
	t.Cleanup(func() { atomicRename = old })

	err := writeFileAtomic(path, []byte("new\n"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("expected rename failure, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("original changed: %q", got)
	}
}

func TestWriteFileAtomicRestoresOriginalWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	calls := 0
	// Fail only temp→target (2nd call). Let target→bak and bak→target succeed
	// so restore of the original content can be verified.
	atomicRename = func(oldpath, newpath string) error {
		calls++
		if calls == 2 {
			return errors.New("replace failed")
		}
		return old(oldpath, newpath)
	}
	t.Cleanup(func() { atomicRename = old })

	err := writeFileAtomic(path, []byte("new\n"), 0o600)
	if err == nil || !strings.Contains(err.Error(), "replace failed") {
		t.Fatalf("expected replace failure, got %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("original not restored: %q", got)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bak should be cleaned or restored, stat=%v", err)
	}
}
