package handler

import "testing"

func TestAndroidSupportedRecognizesMagiskEnvMarker(t *testing.T) {
	t.Setenv("PANEL_MAGISK_MODULE", "1")

	if !androidSupported() {
		t.Fatal("expected androidSupported to recognize Magisk env marker")
	}
}

func TestResolveAndroidRuntimeBinDirPrefersEnvOverride(t *testing.T) {
	t.Setenv("PANEL_ANDROID_RUNTIME_BIN_DIR", "/data/adb/panel/custom-bin")

	got := resolveAndroidRuntimeBinDir()
	if got != "/data/adb/panel/custom-bin" {
		t.Fatalf("expected env override bin dir, got %q", got)
	}
}

func TestResolveAndroidRuntimeBinDirFallsBackToDefault(t *testing.T) {
	got := resolveAndroidRuntimeBinDir()
	if got != defaultAndroidRuntimeBinDir {
		t.Fatalf("expected default android runtime bin dir %q, got %q", defaultAndroidRuntimeBinDir, got)
	}
}
