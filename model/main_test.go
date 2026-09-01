package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRunMigrationsRepairsDuplicatesAndCreatesUniqueIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Provider{}, &ProviderToken{}, &ModelRoute{}, &LLMTrace{}); err != nil {
		t.Fatal(err)
	}
	DB = db
	if err := db.Create(&Provider{Id: 1, Name: "provider"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ProviderToken{Id: 1, ProviderId: 1, UpstreamTokenId: 9, GroupName: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ProviderToken{Id: 2, ProviderId: 1, UpstreamTokenId: 9, GroupName: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ModelRoute{Id: 1, ProviderId: 1, ProviderTokenId: 1, ModelName: "duplicate"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ModelRoute{Id: 2, ProviderId: 1, ProviderTokenId: 1, ModelName: "duplicate"}).Error; err != nil {
		t.Fatal(err)
	}

	if err := runMigrations(db); err != nil {
		t.Fatal(err)
	}

	var tokenCount int64
	if err := db.Model(&ProviderToken{}).Where("provider_id = ? AND upstream_token_id = ?", 1, 9).Count(&tokenCount).Error; err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 {
		t.Fatalf("expected one token after repair, got %d", tokenCount)
	}
	var routeCount int64
	if err := db.Model(&ModelRoute{}).Where("provider_id = ? AND provider_token_id = ? AND model_name = ?", 1, 1, "duplicate").Count(&routeCount).Error; err != nil {
		t.Fatal(err)
	}
	if routeCount != 1 {
		t.Fatalf("expected one route after repair, got %d", routeCount)
	}
	if !db.Migrator().HasIndex(&ModelRoute{}, "idx_model_routes_provider_token_model") {
		t.Fatal("route uniqueness index was not created")
	}
	if !db.Migrator().HasIndex(&ProviderToken{}, "idx_provider_tokens_provider_upstream") {
		t.Fatal("token uniqueness index was not created")
	}
}
