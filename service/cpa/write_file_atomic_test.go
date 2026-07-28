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

func TestWriteFileAtomicReplacesExistingInSingleStep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	calls := 0
	atomicRename = func(oldpath, newpath string) error {
		calls++
		if filepath.Base(newpath) == "cred.json.bak" {
			t.Fatalf("writeFileAtomic moved original to backup before replacing")
		}
		return old(oldpath, newpath)
	}
	t.Cleanup(func() { atomicRename = old })

	if err := writeFileAtomic(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	if calls != 1 {
		t.Fatalf("replace calls = %d, want 1", calls)
	}
}

func TestWriteFileAtomicLeavesOriginalWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	atomicRename = func(string, string) error { return errors.New("replace failed") }
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
		t.Fatalf("original changed: %q", got)
	}
}

func TestWriteFileAtomicCleansTempWhenReplaceFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cred.json")
	original := []byte("keep-me\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	old := atomicRename
	atomicRename = func(string, string) error { return errors.New("replace failed") }
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
		t.Fatalf("original changed: %q", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".cred.json.tmp-") || entry.Name() == "cred.json.bak" {
			t.Fatalf("leftover replacement file: %s", entry.Name())
		}
	}
}
