package service

import (
	"NewAPI-Gateway/model"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupCPAProviderTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	model.DB = db
	if err := db.AutoMigrate(&model.Provider{}, &model.ProviderToken{}, &model.ModelRoute{}, &model.ModelPricing{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestUpsertEmbeddedCPAProviderIdempotent(t *testing.T) {
	setupCPAProviderTestDB(t)

	// First registration inserts a new key_only provider.
	p1, err := upsertEmbeddedCPAProvider("http://127.0.0.1:18317", "key-one")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if p1 == nil || p1.Id == 0 {
		t.Fatalf("expected a persisted provider, got %+v", p1)
	}
	if p1.ProviderType != model.ProviderTypeKeyOnly {
		t.Fatalf("provider type = %q, want %q", p1.ProviderType, model.ProviderTypeKeyOnly)
	}
	if p1.CheckinEnabled {
		t.Fatalf("embedded CPA provider must not have checkin enabled")
	}

	// Simulate an operator tuning priority/weight after registration.
	p1.Priority = 5
	p1.Weight = 42
	if err := p1.Update(); err != nil {
		t.Fatalf("operator update: %v", err)
	}

	// Second registration with new connection details must update in place,
	// preserving the operator-tuned fields and not creating a duplicate.
	p2, err := upsertEmbeddedCPAProvider("http://127.0.0.1:29000", "key-two")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if p2.Id != p1.Id {
		t.Fatalf("expected same provider id (idempotent), got %d then %d", p1.Id, p2.Id)
	}
	if p2.BaseURL != "http://127.0.0.1:29000" {
		t.Fatalf("base url not updated: %s", p2.BaseURL)
	}
	if p2.ApiKey != "key-two" {
		t.Fatalf("api key not updated: %s", p2.ApiKey)
	}
	if p2.Priority != 5 || p2.Weight != 42 {
		t.Fatalf("operator tuning not preserved: priority=%d weight=%d", p2.Priority, p2.Weight)
	}

	// Only one provider with the well-known name should exist.
	var count int64
	if err := model.DB.Model(&model.Provider{}).Where("name = ?", EmbeddedCPAProviderName).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 embedded CPA provider, got %d", count)
	}
}
