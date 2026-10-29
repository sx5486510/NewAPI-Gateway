package model

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestBuildSQLiteDSNAppendsPragmas(t *testing.T) {
	plain := buildSQLiteDSN("gateway.db")
	if plain != "gateway.db?_busy_timeout=5000&_synchronous=NORMAL&_journal_mode=WAL" {
		t.Fatalf("unexpected plain dsn: %s", plain)
	}
	withParams := buildSQLiteDSN("file:gw.db?cache=shared")
	if withParams != "file:gw.db?cache=shared&_busy_timeout=5000&_synchronous=NORMAL&_journal_mode=WAL" {
		t.Fatalf("unexpected dsn with existing params: %s", withParams)
	}
}

func TestConfigureSQLiteConnectionAppliesPool(t *testing.T) {
	t.Setenv("SQL_MAX_OPEN_CONNS", "")
	dbPath := filepath.Join(t.TempDir(), "pool.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := configureSQLiteConnection(db); err != nil {
		t.Fatalf("configure sqlite: %v", err)
	}
	if got := sqlDB.Stats().MaxOpenConnections; got != sqliteMaxOpenConnsDefault {
		t.Fatalf("expected max open conns %d, got %d", sqliteMaxOpenConnsDefault, got)
	}
}

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
