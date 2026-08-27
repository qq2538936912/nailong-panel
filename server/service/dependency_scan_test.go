package service

import (
	"os"
	"path/filepath"
	"testing"

	"panel/config"
	"panel/database"
	"panel/model"
	"panel/testutil"
)

func TestScanMissingDependenciesFromScriptDepeAndManifests(t *testing.T) {
	testutil.SetupTestEnv(t)

	scriptsDir := config.C.Data.ScriptsDir
	if err := os.MkdirAll(filepath.Join(scriptsDir, "bundle"), 0o755); err != nil {
		t.Fatalf("mkdir scripts bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "bundle", "api.js"), []byte(`/**
 * @depe axios, cheerio
 */
export default async function main() {}`), 0o644); err != nil {
		t.Fatalf("write api.js: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "bundle", "task.py"), []byte(`# [depe: ["requests", "./helper.py"]]
import os`), 0o644); err != nil {
		t.Fatalf("write task.py: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "requirements.txt"), []byte("flask>=2.0\n# comment\n"), 0o644); err != nil {
		t.Fatalf("write requirements.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "package.json"), []byte(`{"dependencies":{"dayjs":"^1.11.0"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	result, err := ScanMissingDependencies(scriptsDir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.ScannedFiles < 4 {
		t.Fatalf("expected at least 4 scanned files, got %d", result.ScannedFiles)
	}

	pythonNames := map[string]ScannedDependencyItem{}
	for _, item := range result.Python {
		pythonNames[item.Name] = item
	}
	for _, want := range []string{"requests", "flask>=2.0"} {
		if _, ok := pythonNames[want]; !ok {
			t.Fatalf("expected python dependency %q in scan result, got %#v", want, result.Python)
		}
	}

	nodeNames := map[string]ScannedDependencyItem{}
	for _, item := range result.NodeJS {
		nodeNames[item.Name] = item
	}
	for _, want := range []string{"axios", "cheerio", "dayjs@^1.11.0"} {
		if _, ok := nodeNames[want]; !ok {
			t.Fatalf("expected node dependency %q in scan result, got %#v", want, result.NodeJS)
		}
	}

	if len(result.LocalModules) != 1 || result.LocalModules[0].Path != "./helper.py" {
		t.Fatalf("expected one local module ./helper.py, got %#v", result.LocalModules)
	}
}

func TestScanMissingDependenciesMarksInstalledPython(t *testing.T) {
	testutil.SetupTestEnv(t)

	scriptsDir := config.C.Data.ScriptsDir
	if err := os.WriteFile(filepath.Join(scriptsDir, "only.py"), []byte(`# [depe: requests]`), 0o644); err != nil {
		t.Fatalf("write only.py: %v", err)
	}

	if err := database.DB.Create(&model.Dependency{
		Type:          model.DepTypePython,
		Name:          "requests",
		PythonVersion: DefaultPythonVersion(),
		Status:        model.DepStatusInstalling,
	}).Error; err != nil {
		t.Fatalf("seed dependency: %v", err)
	}

	result, err := ScanMissingDependencies(scriptsDir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	for _, item := range result.Python {
		if item.Name == "requests" && !item.Installed {
			t.Fatalf("requests should be treated as satisfied while installing")
		}
	}
}
