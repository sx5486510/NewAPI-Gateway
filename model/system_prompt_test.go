package model

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupSystemPromptTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	DB = db
	if err := DB.AutoMigrate(&SystemPrompt{}, &ModelRoute{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
}

func TestCreateSystemPromptNormalizesAndEnforcesModelScopedName(t *testing.T) {
	setupSystemPromptTestDB(t)

	first := &SystemPrompt{Name: " general ", ModelName: " gpt-4 ", Content: " Be helpful. "}
	if err := CreateSystemPrompt(first); err != nil {
		t.Fatalf("create first prompt: %v", err)
	}
	if first.Id == 0 || first.Name != "general" || first.ModelName != "gpt-4" || first.Content != "Be helpful." {
		t.Fatalf("prompt was not persisted and normalized: %+v", first)
	}
	if first.CreatedAt == 0 || first.UpdatedAt == 0 {
		t.Fatalf("timestamps were not set: %+v", first)
	}

	if err := CreateSystemPrompt(&SystemPrompt{Name: "general", ModelName: "claude-3", Content: "Helpful"}); err != nil {
		t.Fatalf("same name for different model should succeed: %v", err)
	}
	if err := CreateSystemPrompt(&SystemPrompt{Name: "general", ModelName: "gpt-4", Content: "Duplicate"}); err == nil {
		t.Fatal("duplicate model/name should fail")
	}
	if err := CreateSystemPrompt(&SystemPrompt{Name: " ", ModelName: "gpt-4", Content: "x"}); err == nil {
		t.Fatal("empty normalized name should fail")
	}
}

func TestListSystemPromptsIncludesRouteCount(t *testing.T) {
	setupSystemPromptTestDB(t)
	prompt := &SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "content"}
	if err := CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if err := DB.Create(&ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: i, SystemPromptId: &prompt.Id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	items, err := ListSystemPrompts("gpt-4", "mai")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].RouteCount != 2 {
		t.Fatalf("unexpected list result: %+v", items)
	}
}

func TestUpdateAndGetSystemPromptNormalizeValues(t *testing.T) {
	setupSystemPromptTestDB(t)
	prompt := &SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "old"}
	if err := CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	prompt.Name = " revised "
	prompt.ModelName = " gpt-4o "
	prompt.Content = " new content "
	if err := UpdateSystemPrompt(prompt); err != nil {
		t.Fatalf("update prompt: %v", err)
	}
	got, err := GetSystemPromptByID(prompt.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "revised" || got.ModelName != "gpt-4o" || got.Content != "new content" || got.UpdatedAt == 0 {
		t.Fatalf("unexpected updated prompt: %+v", got)
	}
	prompt.Content = " "
	if err := UpdateSystemPrompt(prompt); err == nil {
		t.Fatal("empty normalized content should fail")
	}
}

func TestUpdateSystemPromptRejectsBoundModelChange(t *testing.T) {
	setupSystemPromptTestDB(t)
	prompt := &SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "old"}
	if err := CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 1, SystemPromptId: &prompt.Id}).Error; err != nil {
		t.Fatal(err)
	}

	prompt.ModelName = "gpt-4o"
	prompt.Content = "new"
	if err := UpdateSystemPrompt(prompt); !errors.Is(err, ErrSystemPromptModelMismatch) {
		t.Fatalf("expected ErrSystemPromptModelMismatch, got %v", err)
	}
	stored, err := GetSystemPromptByID(prompt.Id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ModelName != "gpt-4" || stored.Content != "old" {
		t.Fatalf("rejected update changed prompt: %+v", stored)
	}
}

func TestUpdateSystemPromptReturnsNotFoundForMissingID(t *testing.T) {
	setupSystemPromptTestDB(t)
	err := UpdateSystemPrompt(&SystemPrompt{Id: 999, Name: "missing", ModelName: "gpt-4", Content: "content"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestUpdateSystemPromptSucceedsWhenExistingRowReportsZeroAffected(t *testing.T) {
	setupSystemPromptTestDB(t)
	prompt := &SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "content"}
	if err := CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}

	const callbackName = "test:force_zero_update_rows_affected"
	if err := DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		tx.Statement.RowsAffected = 0
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}

	if err := UpdateSystemPrompt(prompt); err != nil {
		t.Fatalf("idempotent update of existing prompt: %v", err)
	}
}

func TestDeleteSystemPromptReturnsNotFoundForMissingID(t *testing.T) {
	setupSystemPromptTestDB(t)
	unbound, err := DeleteSystemPrompt(999, true)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
	}
	if unbound != 0 {
		t.Fatalf("unbound = %d, want 0", unbound)
	}
}

func TestDeleteSystemPromptRequiresUnbindAndUnbindsTransactionally(t *testing.T) {
	setupSystemPromptTestDB(t)
	prompt := &SystemPrompt{Name: "main", ModelName: "gpt-4", Content: "content"}
	if err := CreateSystemPrompt(prompt); err != nil {
		t.Fatal(err)
	}
	route := ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 1, SystemPromptId: &prompt.Id}
	if err := DB.Create(&route).Error; err != nil {
		t.Fatal(err)
	}
	referenced, err := DeleteSystemPrompt(prompt.Id, false)
	if !errors.Is(err, ErrSystemPromptInUse) {
		t.Fatalf("expected ErrSystemPromptInUse, got %v", err)
	}
	if referenced != 1 {
		t.Fatalf("referenced route count = %d, want 1", referenced)
	}
	if _, err := GetSystemPromptByID(prompt.Id); err != nil {
		t.Fatalf("prompt should remain after rejected delete: %v", err)
	}

	unbound, err := DeleteSystemPrompt(prompt.Id, true)
	if err != nil {
		t.Fatalf("delete with unbind: %v", err)
	}
	if unbound != 1 {
		t.Fatalf("unbound = %d, want 1", unbound)
	}
	if err := DB.First(&route, route.Id).Error; err != nil {
		t.Fatal(err)
	}
	if route.SystemPromptId != nil {
		t.Fatalf("route remains bound: %+v", route)
	}
	if _, err := GetSystemPromptByID(prompt.Id); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("prompt should be deleted, got %v", err)
	}
}

func TestRebuildRoutesForProviderPreservesSystemPromptBinding(t *testing.T) {
	setupSystemPromptTestDB(t)
	id := 42
	existing := ModelRoute{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 2, Enabled: true, SystemPromptId: &id}
	if err := DB.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if err := RebuildRoutesForProvider(1, []ModelRoute{{ModelName: "gpt-4", ProviderId: 1, ProviderTokenId: 2, Enabled: true}, {ModelName: "gpt-new", ProviderId: 1, ProviderTokenId: 3, Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	var routes []ModelRoute
	if err := DB.Order("id").Find(&routes).Error; err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[0].SystemPromptId == nil || *routes[0].SystemPromptId != id || routes[1].SystemPromptId != nil {
		t.Fatalf("unexpected rebuilt routes: %+v", routes)
	}
}
