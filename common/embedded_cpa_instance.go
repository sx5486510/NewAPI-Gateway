package common

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

const EmbeddedCPAProviderBaseName = "__embedded_cpa__"

var (
	localGatewayInstanceIDMu     sync.Mutex
	localGatewayInstanceIDCached string
	readFileFunc                 = os.ReadFile
	writeFileFunc                = os.WriteFile
	mkdirAllFunc                 = os.MkdirAll
)

func LocalGatewayInstanceID() string {
	localGatewayInstanceIDMu.Lock()
	defer localGatewayInstanceIDMu.Unlock()

	if localGatewayInstanceIDCached != "" {
		return localGatewayInstanceIDCached
	}
	if value := sanitizeGatewayInstanceID(os.Getenv("GATEWAY_INSTANCE_ID")); value != "" {
		localGatewayInstanceIDCached = value
		return localGatewayInstanceIDCached
	}
	if value, err := loadOrCreatePersistentGatewayInstanceID(); err == nil && value != "" {
		localGatewayInstanceIDCached = value
		return localGatewayInstanceIDCached
	}
	hostname, err := os.Hostname()
	if err == nil {
		if value := sanitizeGatewayInstanceID(hostname); value != "" {
			localGatewayInstanceIDCached = value
			return localGatewayInstanceIDCached
		}
	}
	localGatewayInstanceIDCached = "default"
	return localGatewayInstanceIDCached
}

func LocalEmbeddedCPAProviderName() string {
	return EmbeddedCPAProviderNameForInstance(LocalGatewayInstanceID())
}

func EmbeddedCPALogLabel() string {
	return "instance_id=" + LocalGatewayInstanceID() + " provider=" + LocalEmbeddedCPAProviderName()
}

func EmbeddedCPAProviderNameForInstance(instanceID string) string {
	value := sanitizeGatewayInstanceID(instanceID)
	if value == "" {
		value = "default"
	}
	return EmbeddedCPAProviderBaseName + "@" + value
}

func IsEmbeddedCPAProviderName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return trimmed == EmbeddedCPAProviderBaseName || strings.HasPrefix(trimmed, EmbeddedCPAProviderBaseName+"@")
}

func IsLocalEmbeddedCPAProviderName(name string) bool {
	return strings.TrimSpace(name) == LocalEmbeddedCPAProviderName()
}

func sanitizeGatewayInstanceID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	value := strings.Trim(b.String(), "-")
	return value
}

func persistentGatewayInstanceIDPath() string {
	runtimeDir := strings.TrimSpace(os.Getenv("CPA_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = "cpa"
	}
	return filepath.Join(filepath.Clean(runtimeDir), "gateway-instance-id")
}

func loadOrCreatePersistentGatewayInstanceID() (string, error) {
	path := persistentGatewayInstanceIDPath()
	data, err := readFileFunc(path)
	if err == nil {
		if value := sanitizeGatewayInstanceID(string(data)); value != "" {
			return value, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	if err := mkdirAllFunc(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}

	value, err := newPersistentGatewayInstanceID()
	if err != nil {
		return "", err
	}
	if err := writeFileFunc(path, []byte(value+"\n"), 0o600); err != nil {
		if data, readErr := readFileFunc(path); readErr == nil {
			if existing := sanitizeGatewayInstanceID(string(data)); existing != "" {
				return existing, nil
			}
		}
		return "", err
	}
	return value, nil
}

func newPersistentGatewayInstanceID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "gw-" + hex.EncodeToString(raw), nil
}
