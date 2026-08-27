package service

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"panel/config"
	"panel/database"
	"panel/model"
)

const (
	scanRuntimeNode   = "nodejs"
	scanRuntimePython = "python"
)

var (
	scanJSDocBlockPattern     = regexp.MustCompile(`(?s)/\*\*(.*?)\*/`)
	scanPythonDocBlockPattern = regexp.MustCompile(`(?s)(?:"""|''')(.*?)(?:"""|''')`)
	scanDependencyMetaPattern = regexp.MustCompile(`(?m)^\s*(?:\*\s*)?@depe\s+(.+?)\s*$`)
	scanLegacyMetaPattern     = regexp.MustCompile(`(?m)^[ \t]*(?://|#+)[ \t]*\[[ \t]*([\d\w+-]+)[ \t]*:[ \t]*(.*)[ \t]*\][^\r\n]*$`)
	scanInlineMetaPattern     = regexp.MustCompile(`(?m)^[ \t]*(?:\*[ \t]?)?@([\d\w+-]+)(?:[ \t]+([^\r\n]+?))?[ \t]*$`)
)

// ScannedDependencyItem 表示从脚本或清单文件里扫到、且尚未就绪的一条依赖。
type ScannedDependencyItem struct {
	Name      string   `json:"name"`
	Installed bool     `json:"installed"`
	Sources   []string `json:"sources"`
}

// ScannedLocalModuleItem 表示 @depe 里声明的 ./xxx.js 这类本地模块引用（不走 pip/npm）。
type ScannedLocalModuleItem struct {
	Path    string   `json:"path"`
	Sources []string `json:"sources"`
}

// ScanMissingDependenciesResult 汇总脚本目录扫描结果。
type ScanMissingDependenciesResult struct {
	PythonVersion string                    `json:"python_version"`
	ScannedFiles  int                       `json:"scanned_files"`
	Python        []ScannedDependencyItem   `json:"python"`
	NodeJS        []ScannedDependencyItem   `json:"nodejs"`
	LocalModules  []ScannedLocalModuleItem  `json:"local_modules"`
}

type scanDependencyBucket struct {
	items map[string]*ScannedDependencyItem
	order []string
}

type scanLocalModuleBucket struct {
	items map[string]*ScannedLocalModuleItem
	order []string
}

// ScanMissingDependencies 扫描脚本目录里 @depe 声明与 requirements/package.json，找出尚未安装的依赖。
func ScanMissingDependencies(scriptsDir string) (*ScanMissingDependenciesResult, error) {
	scriptsDir = strings.TrimSpace(scriptsDir)
	if scriptsDir == "" {
		scriptsDir = strings.TrimSpace(config.C.Data.ScriptsDir)
	}
	if scriptsDir == "" {
		return nil, os.ErrInvalid
	}

	pythonVersion := DefaultPythonVersion()
	pythonBucket := newScanDependencyBucket()
	nodeBucket := newScanDependencyBucket()
	localModules := newScanLocalModuleBucket()
	scannedFiles := 0

	err := filepath.WalkDir(scriptsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if ShouldIgnoreScriptEntryName(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(scriptsDir, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipScanFile(rel, entry.Name()) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		scannedFiles++

		switch scanManifestKind(entry.Name()) {
		case "requirements":
			for _, spec := range parseRequirementsManifest(content) {
				pythonBucket.add(spec, rel, dependencySatisfied(model.DepTypePython, spec, pythonVersion))
			}
		case "package-json":
			for _, spec := range parsePackageJSONDependencies(content) {
				nodeBucket.add(spec, rel, dependencySatisfied(model.DepTypeNodeJS, spec, ""))
			}
		default:
			runtime := scanRuntimeForScript(entry.Name())
			if runtime == "" {
				return nil
			}
			packages, modules := parseDeclaredScriptDependencies(content, runtime)
			for _, spec := range packages {
				depType := model.DepTypePython
				if runtime == scanRuntimeNode {
					depType = model.DepTypeNodeJS
				}
				pythonVer := pythonVersion
				if depType == model.DepTypeNodeJS {
					pythonVer = ""
				}
				bucket := pythonBucket
				if depType == model.DepTypeNodeJS {
					bucket = nodeBucket
				}
				bucket.add(spec, rel, dependencySatisfied(depType, spec, pythonVer))
			}
			for _, modulePath := range modules {
				localModules.add(modulePath, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ScanMissingDependenciesResult{
		PythonVersion: pythonVersion,
		ScannedFiles:  scannedFiles,
		Python:        pythonBucket.list(),
		NodeJS:        nodeBucket.list(),
		LocalModules:  localModules.list(),
	}, nil
}

func newScanDependencyBucket() *scanDependencyBucket {
	return &scanDependencyBucket{
		items: map[string]*ScannedDependencyItem{},
	}
}

func (b *scanDependencyBucket) add(name, source string, installed bool) {
	name = strings.TrimSpace(name)
	source = strings.TrimSpace(source)
	if name == "" || shouldSkipScannedDependency(name) {
		return
	}
	key := scanDependencyKey(name)
	item, ok := b.items[key]
	if !ok {
		item = &ScannedDependencyItem{Name: name, Installed: installed, Sources: []string{}}
		b.items[key] = item
		b.order = append(b.order, key)
	} else if !installed {
		item.Installed = false
	}
	if source != "" && !containsString(item.Sources, source) {
		item.Sources = append(item.Sources, source)
	}
}

func (b *scanDependencyBucket) list() []ScannedDependencyItem {
	out := make([]ScannedDependencyItem, 0, len(b.order))
	for _, key := range b.order {
		if item, ok := b.items[key]; ok {
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func newScanLocalModuleBucket() *scanLocalModuleBucket {
	return &scanLocalModuleBucket{
		items: map[string]*ScannedLocalModuleItem{},
	}
}

func (b *scanLocalModuleBucket) add(modulePath, source string) {
	modulePath = strings.TrimSpace(modulePath)
	source = strings.TrimSpace(source)
	if modulePath == "" {
		return
	}
	item, ok := b.items[modulePath]
	if !ok {
		item = &ScannedLocalModuleItem{Path: modulePath, Sources: []string{}}
		b.items[modulePath] = item
		b.order = append(b.order, modulePath)
	}
	if source != "" && !containsString(item.Sources, source) {
		item.Sources = append(item.Sources, source)
	}
}

func (b *scanLocalModuleBucket) list() []ScannedLocalModuleItem {
	out := make([]ScannedLocalModuleItem, 0, len(b.order))
	for _, key := range b.order {
		if item, ok := b.items[key]; ok {
			out = append(out, *item)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	return out
}

func shouldSkipScanFile(relPath, baseName string) bool {
	relPath = strings.ToLower(filepath.ToSlash(relPath))
	if strings.Contains(relPath, "/node_modules/") || strings.Contains(relPath, "/__pycache__/") {
		return true
	}
	return strings.HasPrefix(baseName, ".")
}

func scanManifestKind(name string) string {
	lower := strings.ToLower(name)
	switch lower {
	case "requirements.txt":
		return "requirements"
	default:
		if strings.HasPrefix(lower, "requirements-") && strings.HasSuffix(lower, ".txt") {
			return "requirements"
		}
		if lower == "package.json" {
			return "package-json"
		}
	}
	return ""
}

func scanRuntimeForScript(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".py":
		return scanRuntimePython
	case ".js", ".mjs", ".ts":
		return scanRuntimeNode
	default:
		return ""
	}
}

func parseDeclaredScriptDependencies(content, runtime string) (packages []string, modules []string) {
	values := declaredDependencyMetaValues(content)
	packageValues := make([]string, 0, len(values))
	moduleSet := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "./") {
			if module, ok := normalizeDeclaredLocalModule(value, runtime); ok && !moduleSet[module] {
				moduleSet[module] = true
				modules = append(modules, module)
			}
			continue
		}
		packageValues = append(packageValues, value)
	}
	if runtime == scanRuntimePython {
		return normalizePythonScanDependencyNames(packageValues), modules
	}
	return normalizeNodeScanDependencyNames(packageValues), modules
}

func declaredDependencyMetaValues(content string) []string {
	values := []string{}
	for _, match := range scanLegacyMetaPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(match[1]), "depe") {
			continue
		}
		values = appendScanDependencyMetaValues(values, match[2])
	}
	for _, match := range scanInlineMetaPattern.FindAllStringSubmatch(content, -1) {
		if len(match) < 3 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(match[1]), "depe") {
			continue
		}
		values = appendScanDependencyMetaValues(values, match[2])
	}
	block := ""
	if match := scanJSDocBlockPattern.FindStringSubmatch(content); len(match) > 1 {
		block = match[1]
	}
	if block == "" {
		if match := scanPythonDocBlockPattern.FindStringSubmatch(content); len(match) > 1 {
			block = match[1]
		}
	}
	for _, match := range scanDependencyMetaPattern.FindAllStringSubmatch(block, -1) {
		if len(match) < 2 {
			continue
		}
		values = appendScanDependencyMetaValues(values, match[1])
	}
	return values
}

func appendScanDependencyMetaValues(values []string, raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return values
	}
	list := []string{}
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		return append(values, list...)
	}
	manifest := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &manifest); err == nil {
		for name := range manifest {
			values = append(values, name)
		}
		return values
	}
	for _, item := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ' ' || r == '\t'
	}) {
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		if item != "" {
			values = append(values, item)
		}
	}
	return values
}

func normalizeDeclaredLocalModule(value, runtime string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if !strings.HasPrefix(value, "./") || strings.Contains(value, "..") || strings.Contains(value[2:], "/") {
		return "", false
	}
	name := filepath.Base(value)
	ext := strings.ToLower(filepath.Ext(name))
	wantExt := ".js"
	if runtime == scanRuntimePython {
		wantExt = ".py"
	}
	if ext != wantExt || strings.TrimSuffix(name, ext) == "" {
		return "", false
	}
	return value, true
}

func normalizePythonScanDependencyNames(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		spec := normalizePythonScanDependencySpec(value)
		if spec == "" {
			continue
		}
		key := CanonicalizePythonPackageName(spec)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}

func normalizePythonScanDependencySpec(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, ".") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "node:") {
		return ""
	}
	if value == "panel" {
		return ""
	}
	return value
}

func normalizeNodeScanDependencyNames(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		spec := normalizeNodeScanDependencySpec(value)
		if spec == "" {
			continue
		}
		key := strings.ToLower(NormalizeNodeDependencyPackageName(spec))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, spec)
	}
	sort.Strings(out)
	return out
}

func normalizeNodeScanDependencySpec(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, ".") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "node:") {
		return ""
	}
	name := NormalizeNodeDependencyPackageName(value)
	if name == "" {
		return ""
	}
	return value
}

func parseRequirementsManifest(content string) []string {
	items := []string{}
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "//") {
			continue
		}
		if idx := strings.Index(raw, " #"); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
		if raw == "" || shouldSkipScannedDependency(raw) {
			continue
		}
		key := CanonicalizePythonPackageName(raw)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, raw)
	}
	sort.Strings(items)
	return items
}

func parsePackageJSONDependencies(content string) []string {
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil {
		return nil
	}
	specs := []string{}
	seen := map[string]bool{}
	for section, deps := range []map[string]string{manifest.Dependencies, manifest.DevDependencies} {
		_ = section
		for name, version := range deps {
			name = strings.TrimSpace(name)
			if name == "" || shouldSkipScannedDependency(name) {
				continue
			}
			spec := name
			version = strings.TrimSpace(version)
			if version != "" && version != "*" {
				spec = name + "@" + version
			}
			key := strings.ToLower(NormalizeNodeDependencyPackageName(spec))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			specs = append(specs, spec)
		}
	}
	sort.Strings(specs)
	return specs
}

func shouldSkipScannedDependency(name string) bool {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return true
	}
	switch CanonicalizePythonPackageName(key) {
	case "panel", "grpcio", "protobuf":
		return true
	}
	switch strings.ToLower(NormalizeNodeDependencyPackageName(key)) {
	case "@grpc/grpc-js", "google-protobuf":
		return true
	}
	return false
}

func scanDependencyKey(name string) string {
	if strings.Contains(name, "@") && !strings.HasPrefix(name, "@") {
		return strings.ToLower(NormalizeNodeDependencyPackageName(name))
	}
	if strings.HasPrefix(name, "@") {
		return strings.ToLower(NormalizeNodeDependencyPackageName(name))
	}
	if canonical := CanonicalizePythonPackageName(name); canonical != "" {
		return "py:" + canonical
	}
	return strings.ToLower(NormalizeNodeDependencyPackageName(name))
}

func dependencySatisfied(depType, name, pythonVersion string) bool {
	if DependencyInstalledForPythonVersion(depType, name, pythonVersion) {
		return true
	}
	return dependencyInstallPending(depType, name, pythonVersion)
}

func dependencyInstallPending(depType, name, pythonVersion string) bool {
	query := database.DB.Model(&model.Dependency{}).Where("type = ? AND name = ?", depType, name)
	if depType == model.DepTypePython {
		query = query.Where("python_version = ?", NormalizeDependencyPythonVersion(pythonVersion))
	}
	var count int64
	query.Where("status IN ?", []string{model.DepStatusInstalling, model.DepStatusQueued}).Count(&count)
	return count > 0
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
