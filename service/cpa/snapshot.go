package cpa

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"

	cpaconfig "github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const snapshotOptionKey = "CPAConfigYAML"

type optionStore interface {
	Get(key string) string
	Set(key, value string) error
}

type databaseOptionStore struct{}

func (databaseOptionStore) Get(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

func (databaseOptionStore) Set(key, value string) error {
	return model.UpdateOption(key, value)
}

type RuntimeInvariants struct {
	managementSentinelHash string
}

func NewRuntimeInvariants(randomSource io.Reader) (*RuntimeInvariants, error) {
	if randomSource == nil {
		randomSource = rand.Reader
	}
	raw := make([]byte, 32)
	if _, err := io.ReadFull(randomSource, raw); err != nil {
		return nil, fmt.Errorf("cpa: generate management sentinel: %w", err)
	}
	encoded := make([]byte, base64.RawURLEncoding.EncodedLen(len(raw)))
	base64.RawURLEncoding.Encode(encoded, raw)
	clear(raw)
	hash, err := bcrypt.GenerateFromPassword(encoded, bcrypt.DefaultCost)
	clear(encoded)
	if err != nil {
		return nil, fmt.Errorf("cpa: hash management sentinel: %w", err)
	}
	return &RuntimeInvariants{managementSentinelHash: string(hash)}, nil
}

func (i *RuntimeInvariants) ApplyYAML(raw []byte) ([]byte, *cpaconfig.Config, error) {
	return i.applyYAML(raw, 0)
}

func (i *RuntimeInvariants) applyYAML(raw []byte, fixedPort int) ([]byte, *cpaconfig.Config, error) {
	if i == nil || strings.TrimSpace(i.managementSentinelHash) == "" {
		return nil, nil, fmt.Errorf("cpa: runtime invariants are not initialized")
	}
	document, root, err := parseYAMLDocument(raw)
	if err != nil {
		return nil, nil, err
	}
	setScalar(root, "host", "!!str", loopbackHost)
	if fixedPort > 0 {
		setScalar(root, "port", "!!int", strconv.Itoa(fixedPort))
	}
	remote, err := ensureMapping(root, "remote-management")
	if err != nil {
		return nil, nil, err
	}
	// All remote-management fields are operator-controlled. The gateway only
	// constrains the bind host and (after startup) the internal port.
	if _, ok := mappingValue(remote, "secret-key"); !ok {
		setScalar(remote, "secret-key", "!!str", i.managementSentinelHash)
	}

	normalized, err := encodeYAMLDocument(document)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := cpaconfig.ParseConfigBytes(normalized)
	if err != nil {
		return nil, nil, fmt.Errorf("cpa: validate config: %w", err)
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, nil, fmt.Errorf("cpa: port must be between 1 and 65535")
	}
	return normalized, cfg, nil
}

type SnapshotStore struct {
	mu         sync.Mutex
	options    optionStore
	runtimeDir string
	invariants *RuntimeInvariants
	fixedPort  int
	renameFile func(oldPath, newPath string) error
}

func NewSnapshotStore(runtimeDir string, invariants *RuntimeInvariants) *SnapshotStore {
	if strings.TrimSpace(runtimeDir) == "" {
		runtimeDir = strings.TrimSpace(os.Getenv("CPA_RUNTIME_DIR"))
	}
	if strings.TrimSpace(runtimeDir) == "" {
		runtimeDir = "cpa"
	}
	return &SnapshotStore{
		options:    databaseOptionStore{},
		runtimeDir: filepath.Clean(runtimeDir),
		invariants: invariants,
		renameFile: os.Rename,
	}
}

func (s *SnapshotStore) Path() string {
	return filepath.Join(s.runtimeDir, "config.yaml")
}

func (s *SnapshotStore) applyYAML(raw []byte) ([]byte, *cpaconfig.Config, error) {
	normalized, cfg, err := s.invariants.applyYAML(raw, s.fixedPort)
	if err == nil && s.fixedPort == 0 {
		s.fixedPort = cfg.Port
	}
	return normalized, cfg, err
}

func (s *SnapshotStore) LoadOrMigrate() ([]byte, *cpaconfig.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadOrMigrateLocked()
}

func (s *SnapshotStore) loadOrMigrateLocked() ([]byte, *cpaconfig.Config, error) {
	databaseRaw := []byte(s.options.Get(snapshotOptionKey))
	diskRaw, diskErr := os.ReadFile(s.Path())
	diskExists := diskErr == nil
	if diskErr != nil && !errors.Is(diskErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("cpa: read runtime config: %w", diskErr)
	}

	var invalidDisk error
	if diskExists {
		normalized, cfg, err := s.applyYAML(diskRaw)
		if err == nil {
			if err := s.writeAtomic(normalized); err != nil {
				return nil, nil, err
			}
			if string(databaseRaw) != string(normalized) {
				if err := s.options.Set(snapshotOptionKey, string(normalized)); err != nil {
					return nil, nil, fmt.Errorf("cpa: persist recovered runtime snapshot: %w", err)
				}
			}
			return normalized, cfg, nil
		}
		invalidDisk = err
	}

	if len(bytes.TrimSpace(databaseRaw)) > 0 {
		normalized, cfg, err := s.applyYAML(databaseRaw)
		if err == nil {
			if err := s.writeAtomic(normalized); err != nil {
				return nil, nil, err
			}
			if string(databaseRaw) != string(normalized) {
				if err := s.options.Set(snapshotOptionKey, string(normalized)); err != nil {
					return nil, nil, fmt.Errorf("cpa: persist normalized database snapshot: %w", err)
				}
			}
			return normalized, cfg, nil
		}
		if invalidDisk != nil {
			return nil, nil, fmt.Errorf("cpa: invalid runtime config (%v) and database snapshot (%w)", invalidDisk, err)
		}
		return nil, nil, fmt.Errorf("cpa: invalid database snapshot: %w", err)
	}

	if invalidDisk != nil {
		return nil, nil, fmt.Errorf("cpa: invalid runtime config and no database snapshot: %w", invalidDisk)
	}

	legacy, err := s.legacyConfig()
	if err != nil {
		return nil, nil, err
	}
	seed, err := yaml.Marshal(struct {
		Host    string   `yaml:"host"`
		Port    int      `yaml:"port"`
		AuthDir string   `yaml:"auth-dir"`
		APIKeys []string `yaml:"api-keys"`
	}{Host: loopbackHost, Port: legacy.Port, AuthDir: legacy.AuthDir, APIKeys: legacy.APIKeys})
	if err != nil {
		return nil, nil, fmt.Errorf("cpa: marshal legacy config: %w", err)
	}
	normalized, cfg, err := s.applyYAML(seed)
	if err != nil {
		return nil, nil, err
	}
	if err := s.writeAtomic(normalized); err != nil {
		return nil, nil, err
	}
	if err := s.options.Set(snapshotOptionKey, string(normalized)); err != nil {
		return nil, nil, fmt.Errorf("cpa: persist migrated snapshot: %w", err)
	}
	return normalized, cfg, nil
}

func (s *SnapshotStore) PersistRuntime() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.Path())
	if err != nil {
		return fmt.Errorf("cpa: read runtime config for persistence: %w", err)
	}
	normalized, _, err := s.applyYAML(raw)
	if err != nil {
		return err
	}
	if err := s.writeAtomic(normalized); err != nil {
		return err
	}
	if err := s.options.Set(snapshotOptionKey, string(normalized)); err != nil {
		return fmt.Errorf("cpa: persist runtime snapshot: %w", err)
	}
	return nil
}

func (s *SnapshotStore) PatchBasic(next CPAConfig) error {
	if err := validateBasicConfig(&next); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _, err := s.loadOrMigrateLocked()
	if err != nil {
		return err
	}
	document, root, err := parseYAMLDocument(raw)
	if err != nil {
		return err
	}
	setScalar(root, "port", "!!int", strconv.Itoa(next.Port))
	setScalar(root, "auth-dir", "!!str", strings.TrimSpace(next.AuthDir))
	setStringSequence(root, "api-keys", next.APIKeys)
	patched, err := encodeYAMLDocument(document)
	if err != nil {
		return err
	}
	normalized, _, err := s.applyYAML(patched)
	if err != nil {
		return err
	}
	if err := s.writeAtomic(normalized); err != nil {
		return err
	}
	if err := s.options.Set(snapshotOptionKey, string(normalized)); err != nil {
		return fmt.Errorf("cpa: persist patched snapshot: %w", err)
	}
	enabled := "false"
	if next.Enabled {
		enabled = "true"
	}
	if err := s.options.Set("CPAEnabled", enabled); err != nil {
		return fmt.Errorf("cpa: persist CPAEnabled: %w", err)
	}
	return nil
}

func (s *SnapshotStore) Basic() (*CPAConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, cfg, err := s.loadOrMigrateLocked()
	if err != nil {
		return nil, err
	}
	return &CPAConfig{
		Enabled: strings.EqualFold(strings.TrimSpace(s.options.Get("CPAEnabled")), "true"),
		APIKeys: append([]string(nil), cfg.APIKeys...),
		AuthDir: cfg.AuthDir,
		Port:    cfg.Port,
	}, nil
}

func (s *SnapshotStore) legacyConfig() (*CPAConfig, error) {
	enabled := strings.EqualFold(strings.TrimSpace(s.options.Get("CPAEnabled")), "true")
	apiKeys := []string{"cpa-default-key"}
	rawKeys := strings.TrimSpace(s.options.Get("CPAAPIKeys"))
	if rawKeys != "" {
		if err := json.Unmarshal([]byte(rawKeys), &apiKeys); err != nil {
			return nil, fmt.Errorf("cpa: parse CPAAPIKeys JSON: %w", err)
		}
	}
	authDir := strings.TrimSpace(s.options.Get("CPAAuthDir"))
	if authDir == "" {
		authDir = "~/.cli-proxy-api"
	}
	port, err := strconv.Atoi(strings.TrimSpace(s.options.Get("CPAPort")))
	if err != nil || port <= 0 || port > 65535 {
		port = 18317
	}
	legacy := &CPAConfig{Enabled: enabled, APIKeys: apiKeys, AuthDir: authDir, Port: port}
	if err := validateBasicConfig(legacy); err != nil {
		return nil, err
	}
	return legacy, nil
}

func validateBasicConfig(cfg *CPAConfig) error {
	if cfg == nil {
		return fmt.Errorf("cpa: config is nil")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return fmt.Errorf("cpa: port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.AuthDir) == "" {
		return fmt.Errorf("cpa: auth dir is empty")
	}
	if len(cfg.APIKeys) == 0 {
		return fmt.Errorf("cpa: at least one API key is required")
	}
	for index := range cfg.APIKeys {
		cfg.APIKeys[index] = strings.TrimSpace(cfg.APIKeys[index])
		if cfg.APIKeys[index] == "" {
			return fmt.Errorf("cpa: API key %d is empty", index)
		}
	}
	return nil
}

func (s *SnapshotStore) writeAtomic(data []byte) (err error) {
	if err := os.MkdirAll(s.runtimeDir, 0o700); err != nil {
		return fmt.Errorf("cpa: create runtime dir: %w", err)
	}
	if err := os.Chmod(s.runtimeDir, 0o700); err != nil {
		return fmt.Errorf("cpa: secure runtime dir: %w", err)
	}
	tempPath := s.Path() + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cpa: create temporary config: %w", err)
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err = file.Chmod(0o600); err != nil {
		return fmt.Errorf("cpa: secure temporary config: %w", err)
	}
	if _, err = file.Write(data); err != nil {
		return fmt.Errorf("cpa: write temporary config: %w", err)
	}
	if err = file.Sync(); err != nil {
		return fmt.Errorf("cpa: sync temporary config: %w", err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("cpa: close temporary config: %w", err)
	}

	// Windows does not allow rename onto an existing file, so we cannot simply
	// rename the temp file over the target. Rather than deleting the target
	// first (which loses the original if the rename then fails), move the
	// original aside to a backup, rename the temp file into place, and only
	// then drop the backup. If the rename fails, restore the backup so the
	// original config survives.
	targetPath := s.Path()
	backupPath := targetPath + ".bak"
	haveBackup := false
	if _, statErr := os.Stat(targetPath); statErr == nil {
		_ = os.Remove(backupPath)
		if err = s.renameFile(targetPath, backupPath); err != nil {
			return fmt.Errorf("cpa: back up old config before replace: %w", err)
		}
		haveBackup = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		err = statErr
		return fmt.Errorf("cpa: stat old config before replace: %w", err)
	}

	if err = s.renameFile(tempPath, targetPath); err != nil {
		if haveBackup {
			// Restore the original; report the restore failure too if it happens.
			if restoreErr := s.renameFile(backupPath, targetPath); restoreErr != nil {
				return fmt.Errorf("cpa: replace runtime config: %w (and restoring original failed: %v)", err, restoreErr)
			}
		}
		return fmt.Errorf("cpa: replace runtime config: %w", err)
	}

	if haveBackup {
		_ = os.Remove(backupPath)
	}
	return nil
}

func parseYAMLDocument(raw []byte) (*yaml.Node, *yaml.Node, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil, fmt.Errorf("cpa: config payload is empty")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, nil, fmt.Errorf("cpa: parse YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return nil, nil, fmt.Errorf("cpa: multiple YAML documents are not allowed")
	} else if !errors.Is(err, io.EOF) {
		return nil, nil, fmt.Errorf("cpa: parse trailing YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("cpa: YAML root must be a mapping")
	}
	if err := validateYAMLNode(document.Content[0]); err != nil {
		return nil, nil, err
	}
	return &document, document.Content[0], nil
}

func validateYAMLNode(node *yaml.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("cpa: YAML aliases are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if _, exists := seen[key]; exists {
				return fmt.Errorf("cpa: duplicate YAML key %q", key)
			}
			seen[key] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateYAMLNode(child); err != nil {
			return err
		}
	}
	return nil
}

func encodeYAMLDocument(document *yaml.Node) ([]byte, error) {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, fmt.Errorf("cpa: encode YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("cpa: close YAML encoder: %w", err)
	}
	return output.Bytes(), nil
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1], true
		}
	}
	return nil, false
}

func setScalar(mapping *yaml.Node, key, tag, value string) {
	if existing, ok := mappingValue(mapping, key); ok {
		existing.Kind = yaml.ScalarNode
		existing.Tag = tag
		existing.Value = value
		existing.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}

func ensureMapping(mapping *yaml.Node, key string) (*yaml.Node, error) {
	if existing, ok := mappingValue(mapping, key); ok {
		if existing.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("cpa: YAML key %q must be a mapping", key)
		}
		return existing, nil
	}
	value := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
	return value, nil
}

func setStringSequence(mapping *yaml.Node, key string, values []string) {
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.TrimSpace(value)})
	}
	if existing, ok := mappingValue(mapping, key); ok {
		head, line, foot := existing.HeadComment, existing.LineComment, existing.FootComment
		*existing = *sequence
		existing.HeadComment, existing.LineComment, existing.FootComment = head, line, foot
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		sequence,
	)
}
