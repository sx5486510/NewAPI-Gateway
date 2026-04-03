package service

import (
	"NewAPI-Gateway/model"
	"reflect"
	"testing"
)

func TestCollectRequiredTokenGroups(t *testing.T) {
	pricingList := []*model.ModelPricing{
		{EnableGroups: `["default", "vip"]`},
		{EnableGroups: `["beta", " "]`},
		{EnableGroups: `["vip"]`},
		{EnableGroups: `invalid json`},
	}

	got := collectRequiredTokenGroups(pricingList)
	want := []string{"beta", "default", "vip"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collectRequiredTokenGroups() = %v, want %v", got, want)
	}
}

func TestGetMissingTokenGroups(t *testing.T) {
	requiredGroups := []string{"beta", "default", "vip"}
	tokens := []UpstreamToken{
		{Group: ""},
		{Group: " vip "},
	}

	got := getMissingTokenGroups(requiredGroups, tokens)
	want := []string{"beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("getMissingTokenGroups() = %v, want %v", got, want)
	}
}
