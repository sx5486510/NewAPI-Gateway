package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestParseModelBlocklist(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "whitespace only", raw: "  \n\t ", want: nil},
		{name: "newline separated", raw: "gpt-4-*\n*-preview\n", want: []string{"gpt-4-*", "*-preview"}},
		{name: "comma separated", raw: "GPT-4-*, DALL·E-2", want: []string{"gpt-4-*", "dall·e-2"}},
		{name: "mixed separators and dedup", raw: "Foo\nfoo,\n bar \n", want: []string{"foo", "bar"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseModelBlocklist(tc.raw)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseModelBlocklist(%q) = %#v, want %#v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestIsModelBlocked(t *testing.T) {
	cases := []struct {
		name     string
		patterns []string
		model    string
		want     bool
	}{
		{name: "no patterns", patterns: nil, model: "gpt-4o", want: false},
		{name: "exact match", patterns: []string{"dall-e-2"}, model: "dall-e-2", want: true},
		{name: "exact match case insensitive", patterns: []string{"DALL-E-2"}, model: "DALL-E-2", want: true},
		{name: "no match different name", patterns: []string{"dall-e-2"}, model: "dall-e-3", want: false},
		{name: "prefix wildcard", patterns: []string{"gpt-4*"}, model: "gpt-4o", want: true},
		{name: "prefix wildcard no match", patterns: []string{"gpt-4*"}, model: "gpt-5", want: false},
		{name: "infix wildcard requires separator", patterns: []string{"gpt-4-*"}, model: "gpt-4o", want: false},
		{name: "infix wildcard match", patterns: []string{"gpt-4-*"}, model: "gpt-4-turbo", want: true},
		{name: "suffix wildcard", patterns: []string{"*-preview"}, model: "o3-preview", want: true},
		{name: "suffix wildcard no match", patterns: []string{"*-preview"}, model: "o3-preview-2", want: false},
		{name: "only wildcard matches everything", patterns: []string{"*"}, model: "anything", want: true},
		{name: "double wildcard", patterns: []string{"gpt*4*"}, model: "gpt-4-turbo", want: true},
		{name: "double wildcard overlap rejected", patterns: []string{"a*a"}, model: "a", want: false},
		{name: "empty pattern only matches empty", patterns: []string{""}, model: "gpt-4o", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsModelBlocked(tc.patterns, tc.model)
			if got != tc.want {
				t.Fatalf("IsModelBlocked(%#v, %q) = %v, want %v", tc.patterns, tc.model, got, tc.want)
			}
		})
	}
}

func TestBuildModelAliasSources(t *testing.T) {
	mapping := map[string]string{
		"my-alias":  "DeepSeek-V4.1-Flash",
		" other ":   " deepseek-v4.1-flash ",
		"":          "orphan",
		"orphan-2":  "",
	}
	got := BuildModelAliasSources(mapping)
	want := map[string][]string{
		"deepseek-v4.1-flash": {"my-alias", "other"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildModelAliasSources() = %#v, want %#v", got, want)
	}
}

func TestIsModelBlockedWithAliases(t *testing.T) {
	aliasSources := map[string][]string{
		"deepseek-v4.1-flash": {"my-alias"},
	}
	cases := []struct {
		name     string
		patterns []string
		model    string
		want     bool
	}{
		{name: "direct match unchanged", patterns: []string{"deepseek-v4*"}, model: "deepseek-v4.1-flash", want: true},
		{name: "blocked via alias", patterns: []string{"my-alias"}, model: "deepseek-v4.1-flash", want: true},
		{name: "alias pattern with wildcard", patterns: []string{"my-*"}, model: "deepseek-v4.1-flash", want: true},
		{name: "unrelated alias no match", patterns: []string{"other-alias"}, model: "deepseek-v4.1-flash", want: false},
		{name: "nil alias map behaves as direct", patterns: []string{"deepseek-v4*"}, model: "deepseek-v4.1-flash", want: true},
		{name: "no match at all", patterns: []string{"glm-*"}, model: "deepseek-v4.1-flash", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := aliasSources
			if tc.name == "nil alias map behaves as direct" {
				lookup = nil
			}
			got := IsModelBlockedWithAliases(tc.patterns, tc.model, lookup)
			if got != tc.want {
				t.Fatalf("IsModelBlockedWithAliases(%#v, %q) = %v, want %v", tc.patterns, tc.model, got, tc.want)
			}
		})
	}
}

func TestRebuildProviderRoutesSkipsBlocklistedAliasTargets(t *testing.T) {
	setupRebuildBlocklistTestDB(t)

	provider := &model.Provider{
		Id:                3,
		Name:              "alias-provider",
		Status:            1,
		ModelAliasMapping: `{"my-alias":"deepseek-v4.1-flash"}`,
	}
	if err := model.DB.Create(provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	token := &model.ProviderToken{
		ProviderId:      provider.Id,
		UpstreamTokenId: 9,
		SkKey:           "sk-test",
		Name:            "auto-sync-default",
		GroupName:       "default",
		Status:          1,
	}
	if err := model.DB.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	pricing := &model.ModelPricing{
		ModelName:    "deepseek-v4.1-flash",
		ProviderId:   provider.Id,
		EnableGroups: `["default"]`,
	}
	if err := model.DB.Create(pricing).Error; err != nil {
		t.Fatalf("create pricing: %v", err)
	}

	// Blocking the alias source removes the route of the original name.
	common.OptionMap["RouteModelBlocklist"] = "my-alias"
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("RebuildProviderRoutes() error = %v", err)
	}
	var count int64
	model.DB.Model(&model.ModelRoute{}).Where("provider_id = ?", provider.Id).Count(&count)
	if count != 0 {
		t.Fatalf("route count with blocked alias = %d, want 0", count)
	}

	// Blocking an unrelated pattern leaves the route in place.
	common.OptionMap["RouteModelBlocklist"] = "unrelated-*"
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("RebuildProviderRoutes() error = %v", err)
	}
	model.DB.Model(&model.ModelRoute{}).Where("provider_id = ?", provider.Id).Count(&count)
	if count != 1 {
		t.Fatalf("route count with unrelated blocklist = %d, want 1", count)
	}
}

func setupRebuildBlocklistTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open in-memory sqlite: %v", err)
	}
	model.DB = db
	if err := model.DB.AutoMigrate(&model.Provider{}, &model.ProviderToken{}, &model.ModelPricing{}, &model.ModelRoute{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	previousOption := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		common.OptionMap = previousOption
	})
}

func TestRebuildProviderRoutesSkipsBlocklistedModels(t *testing.T) {
	setupRebuildBlocklistTestDB(t)

	provider := &model.Provider{Id: 1, Name: "test-provider", Status: 1}
	if err := model.DB.Create(provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	token := &model.ProviderToken{
		ProviderId:     provider.Id,
		UpstreamTokenId: 7,
		SkKey:          "sk-test",
		Name:           "auto-sync-default",
		GroupName:      "default",
		Status:         1,
	}
	if err := model.DB.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	for _, name := range []string{"gpt-4o", "gpt-4-turbo", "dall-e-2", "o3-preview"} {
		pricing := &model.ModelPricing{
			ModelName:    name,
			ProviderId:   provider.Id,
			EnableGroups: `["default"]`,
		}
		if err := model.DB.Create(pricing).Error; err != nil {
			t.Fatalf("create pricing %s: %v", name, err)
		}
	}

	// First rebuild without blocklist: every model gets a route.
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("RebuildProviderRoutes() error = %v", err)
	}
	var count int64
	model.DB.Model(&model.ModelRoute{}).Where("provider_id = ?", provider.Id).Count(&count)
	if count != 4 {
		t.Fatalf("route count without blocklist = %d, want 4", count)
	}

	// Enable blocklist and rebuild again: blocked models are removed
	// from the existing route table, not just excluded from new rows.
	// Note: "gpt-4-*" requires the separator, so "gpt-4o" must survive.
	common.OptionMap["RouteModelBlocklist"] = "gpt-4-*\ndall-e-2"
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("RebuildProviderRoutes() with blocklist error = %v", err)
	}

	var names []string
	if err := model.DB.Model(&model.ModelRoute{}).
		Where("provider_id = ?", provider.Id).
		Order("model_name").
		Pluck("model_name", &names).Error; err != nil {
		t.Fatalf("query routes: %v", err)
	}
	want := []string{"gpt-4o", "o3-preview"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("remaining routes = %#v, want %#v", names, want)
	}
}

func TestRebuildProviderRoutesWithEmptyBlocklistKeepsAllModels(t *testing.T) {
	setupRebuildBlocklistTestDB(t)

	provider := &model.Provider{Id: 2, Name: "test-provider-2", Status: 1}
	if err := model.DB.Create(provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	token := &model.ProviderToken{
		ProviderId:      provider.Id,
		UpstreamTokenId: 8,
		SkKey:           "sk-test",
		Name:            "auto-sync-default",
		GroupName:       "default",
		Status:          1,
	}
	if err := model.DB.Create(token).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}
	for _, name := range []string{"gpt-4o", "text-embedding-3-small"} {
		pricing := &model.ModelPricing{
			ModelName:    name,
			ProviderId:   provider.Id,
			EnableGroups: `["default"]`,
		}
		if err := model.DB.Create(pricing).Error; err != nil {
			t.Fatalf("create pricing %s: %v", name, err)
		}
	}

	// Option absent entirely (not just empty) must not filter anything.
	common.OptionMap["RouteModelBlocklist"] = ""
	if err := RebuildProviderRoutes(provider.Id); err != nil {
		t.Fatalf("RebuildProviderRoutes() error = %v", err)
	}
	var count int64
	model.DB.Model(&model.ModelRoute{}).Where("provider_id = ?", provider.Id).Count(&count)
	if count != 2 {
		t.Fatalf("route count with empty blocklist = %d, want 2", count)
	}
}
