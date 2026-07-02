package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupProviderTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}

	DB = db
	if err := DB.AutoMigrate(&Provider{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
}

func TestGetAllProvidersReturnsCompleteProviders(t *testing.T) {
	setupProviderTestDB(t)

	providers := []Provider{
		{Name: "first", BaseURL: "https://one.example", AccessToken: "token-1", ApiKey: "key-1", ProviderType: ProviderTypeFull, Status: 1, Priority: 3, Weight: 5},
		{Name: "second", BaseURL: "https://two.example", AccessToken: "token-2", ApiKey: "key-2", ProviderType: ProviderTypeKeyOnly, Status: 1, Priority: 7, Weight: 11},
		{Name: "duplicate-url", BaseURL: "https://two.example", AccessToken: "token-3", ApiKey: "key-3", ProviderType: ProviderTypeFull, Status: 1, Priority: 13, Weight: 17},
	}
	for i := range providers {
		if err := providers[i].Insert(); err != nil {
			t.Fatalf("insert provider %d: %v", i, err)
		}
	}

	got, err := GetAllProviders(0, 10)
	if err != nil {
		t.Fatalf("get providers: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected all provider rows, got %d", len(got))
	}

	latest := got[0]
	if latest.Id == 0 || latest.Name != "duplicate-url" || latest.AccessToken != "token-3" || latest.ApiKey != "key-3" || latest.Priority != 13 || latest.Weight != 17 {
		t.Fatalf("expected complete latest provider row, got %+v", latest)
	}
}
