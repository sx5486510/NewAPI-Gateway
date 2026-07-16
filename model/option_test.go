package model

import (
	"testing"

	"NewAPI-Gateway/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitOptionMapIncludesCPAConfigYAML(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open("file:option-map-cpa?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	DB = db
	t.Cleanup(func() { DB = originalDB })
	if err := DB.AutoMigrate(&Option{}); err != nil {
		t.Fatal(err)
	}

	InitOptionMap()
	common.OptionMapRWMutex.RLock()
	value, ok := common.OptionMap["CPAConfigYAML"]
	common.OptionMapRWMutex.RUnlock()
	if !ok || value != "" {
		t.Fatalf("CPAConfigYAML default = %q, present=%v", value, ok)
	}
}

func TestUpdateOptionReturnsDatabaseError(t *testing.T) {
	originalDB := DB
	db, err := gorm.Open(sqlite.Open("file:closed-option?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	DB = db
	t.Cleanup(func() { DB = originalDB })

	if err := UpdateOption("CPAConfigYAML", "value"); err == nil {
		t.Fatal("expected database error")
	}
}
