package model

import (
	"NewAPI-Gateway/common"
	"testing"
)

func TestStripSystemReminderOptionUpdatesCommonFlag(t *testing.T) {
	common.StripSystemReminderEnabled = true
	common.OptionMap = map[string]string{}

	updateOptionMap("StripSystemReminderEnabled", "false")
	if common.StripSystemReminderEnabled {
		t.Fatalf("expected StripSystemReminderEnabled false")
	}

	updateOptionMap("StripSystemReminderEnabled", "true")
	if !common.StripSystemReminderEnabled {
		t.Fatalf("expected StripSystemReminderEnabled true")
	}
}