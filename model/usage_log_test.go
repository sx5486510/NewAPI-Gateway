package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUsageLogInsertStoresTokenGroupName(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	DB = db
	if err := DB.AutoMigrate(&UsageLog{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	log := &UsageLog{
		UserId:          1,
		ProviderTokenId: 2,
		TokenGroupName:  "vip",
		RequestId:       "req-usage-group",
		Status:          1,
	}
	if err := log.Insert(); err != nil {
		t.Fatalf("insert usage log: %v", err)
	}

	var stored UsageLog
	if err := DB.First(&stored, "request_id = ?", "req-usage-group").Error; err != nil {
		t.Fatalf("find usage log: %v", err)
	}
	if stored.TokenGroupName != "vip" {
		t.Fatalf("expected token group vip, got %q", stored.TokenGroupName)
	}
}
