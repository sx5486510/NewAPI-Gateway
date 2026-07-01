package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupProviderTokenTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}

	DB = db
	if err := DB.AutoMigrate(&ProviderToken{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestProviderTokenIsClientAllowedAllDisabled(t *testing.T) {
	token := ProviderToken{BlockClients: true}

	if token.IsClientAllowed("codex") {
		t.Fatal("expected codex client to be blocked when all client restrictions are disabled")
	}
	if token.IsClientAllowed("cc") {
		t.Fatal("expected cc client to be blocked when all client restrictions are disabled")
	}
	if !token.IsClientAllowed("") {
		t.Fatal("expected empty client type to remain allowed")
	}
}

func TestProviderTokenUpdateClearsBooleanRestrictionFields(t *testing.T) {
	setupProviderTokenTestDB(t)

	token := ProviderToken{
		ProviderId:     1,
		Name:           "token",
		AllowCodex:     true,
		AllowCC:        true,
		BlockClients:   false,
		Status:         1,
		Weight:         10,
		UnlimitedQuota: true,
	}
	if err := token.Insert(); err != nil {
		t.Fatalf("insert token: %v", err)
	}

	token.AllowCodex = false
	token.AllowCC = false
	token.BlockClients = true
	if err := token.Update(); err != nil {
		t.Fatalf("update token: %v", err)
	}

	var stored ProviderToken
	if err := DB.First(&stored, token.Id).Error; err != nil {
		t.Fatalf("load token: %v", err)
	}
	if stored.AllowCodex {
		t.Fatal("expected allow_codex to be cleared")
	}
	if stored.AllowCC {
		t.Fatal("expected allow_cc to be cleared")
	}
	if !stored.BlockClients {
		t.Fatal("expected block_clients to be saved")
	}
}
