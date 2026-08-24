package model_test

import (
	"testing"

	"panel/model"
	"panel/testutil"
)

func TestInitDefaultConfigsUpgradesLegacyPanelTitle(t *testing.T) {
	testutil.SetupTestEnv(t)

	setRawSystemConfigValue(t, "panel_title", model.LegacyPanelTitle)
	model.InitDefaultConfigs()

	if got := readSystemConfigValue(t, "panel_title"); got != "奶龙面板" {
		t.Fatalf("legacy panel_title should upgrade to 奶龙面板, got %q", got)
	}
}

func TestInitDefaultConfigsKeepsCustomPanelTitle(t *testing.T) {
	testutil.SetupTestEnv(t)

	setRawSystemConfigValue(t, "panel_title", "我的面板")
	model.InitDefaultConfigs()

	if got := readSystemConfigValue(t, "panel_title"); got != "我的面板" {
		t.Fatalf("custom panel_title must be preserved, got %q", got)
	}
}
