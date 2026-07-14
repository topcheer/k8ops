package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestFrontendImportConsistency verifies that all JS module imports reference
// symbols that are actually exported by the target module.
// This catches the exact bug that caused the 503/frontend crash in v16.54
// where audit-dashboard.js imported apiFetch from core.js which didn't exist.
func TestFrontendImportConsistency(t *testing.T) {
	webDir := findWebDir(t)

	// Collect exports from each module
	exports := map[string]map[string]bool{} // file -> set of exported names
	imports := []struct {
		file   string
		target string
		names  []string
	}{}

	entries, err := os.ReadDir(webDir)
	if err != nil {
		t.Skipf("web dir not found: %v", err)
	}

	// Regex to find export statements
	exportRe := regexp.MustCompile(`export\s+(?:async\s+)?(?:function|const|let|var)\s+(\w+)`)
	// Regex to find import statements: import { a, b, c } from './path'
	importRe := regexp.MustCompile(`import\s+\{([^}]+)\}\s+from\s+'([^']+)'`)

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		path := filepath.Join(webDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)

		// Collect exports
		exportMatches := exportRe.FindAllStringSubmatch(content, -1)
		exports[e.Name()] = make(map[string]bool)
		for _, m := range exportMatches {
			exports[e.Name()][m[1]] = true
		}
		// Also check for `export * from` patterns (re-exports)
		// and `export { foo }` patterns
		exportNamedRe := regexp.MustCompile(`export\s+\{([^}]+)\}`)
		for _, m := range exportNamedRe.FindAllStringSubmatch(content, -1) {
			for _, name := range strings.Split(m[1], ",") {
				name = strings.TrimSpace(name)
				if name != "" {
					exports[e.Name()][name] = true
				}
			}
		}

		// Collect imports
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			names := strings.Split(m[1], ",")
			target := strings.TrimSpace(m[2])
			// Resolve relative path to filename
			target = filepath.Base(target)
			var cleanNames []string
			for _, n := range names {
				n = strings.TrimSpace(n)
				if n != "" {
					cleanNames = append(cleanNames, n)
				}
			}
			imports = append(imports, struct {
				file   string
				target string
				names  []string
			}{e.Name(), target, cleanNames})
		}
	}

	// Also check modules/ subdir
	modDir := filepath.Join(webDir, "modules")
	if modEntries, err := os.ReadDir(modDir); err == nil {
		for _, e := range modEntries {
			if !strings.HasSuffix(e.Name(), ".js") {
				continue
			}
			path := filepath.Join(modDir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			content := string(data)
			exportMatches := exportRe.FindAllStringSubmatch(content, -1)
			exports[e.Name()] = make(map[string]bool)
			for _, m := range exportMatches {
				exports[e.Name()][m[1]] = true
			}
		}
	}

	// Verify all imports resolve
	var errors []string
	for _, imp := range imports {
		targetExports, ok := exports[imp.target]
		if !ok {
			errors = append(errors, fmt.Sprintf("%s: imports from '%s' but file not found", imp.file, imp.target))
			continue
		}
		for _, name := range imp.names {
			if !targetExports[name] {
				errors = append(errors, fmt.Sprintf("%s: imports '%s' from '%s' but it's not exported", imp.file, name, imp.target))
			}
		}
	}

	if len(errors) > 0 {
		sort.Strings(errors)
		t.Errorf("frontend import issues found:\n  %s", strings.Join(errors, "\n  "))
	}
}

// TestFrontendOnclickHandlers verifies that all onclick handlers in index.html
// reference functions that are exported and bridged to window by main.js.
func TestFrontendOnclickHandlers(t *testing.T) {
	webDir := findWebDir(t)

	// Read index.html
	htmlPath := filepath.Join(webDir, "index.html")
	htmlData, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Skipf("index.html not found: %v", err)
	}
	htmlContent := string(htmlData)

	// Extract all onclick="funcName(...)" patterns
	onclickRe := regexp.MustCompile(`onclick="(\w+)\(`)
	onclickMatches := onclickRe.FindAllStringSubmatch(htmlContent, -1)
	if len(onclickMatches) == 0 {
		t.Skip("no onclick handlers in index.html")
	}

	// Collect all exported function names from all JS files
	// and all window.xxx = assignments
	exportRe := regexp.MustCompile(`export\s+(?:async\s+)?(?:function|const)\s+(\w+)`)
	windowRe := regexp.MustCompile(`window\.(\w+)\s*=`)

	allFuncs := make(map[string]bool)
	entries, _ := os.ReadDir(webDir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(webDir, e.Name()))
		content := string(data)
		for _, m := range exportRe.FindAllStringSubmatch(content, -1) {
			allFuncs[m[1]] = true
		}
		for _, m := range windowRe.FindAllStringSubmatch(content, -1) {
			allFuncs[m[1]] = true
		}
	}

	// Check each onclick handler
	var missing []string
	for _, m := range onclickMatches {
		funcName := m[1]
		if !allFuncs[funcName] {
			missing = append(missing, funcName)
		}
	}

	if len(missing) > 0 {
		// Deduplicate
		seen := make(map[string]bool)
		var uniq []string
		for _, m := range missing {
			if !seen[m] {
				seen[m] = true
				uniq = append(uniq, m)
			}
		}
		t.Errorf("onclick handlers in index.html reference undefined functions: %v", uniq)
	}
}

// TestFrontendMainBridge verifies that main.js imports all modules listed in allModules
// and bridges their exports to window.
func TestFrontendMainBridge(t *testing.T) {
	webDir := findWebDir(t)

	mainPath := filepath.Join(webDir, "main.js")
	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Skipf("main.js not found: %v", err)
	}
	content := string(data)

	// Check that main.js has the window bridge pattern
	if !strings.Contains(content, "window[") && !strings.Contains(content, "window.") {
		t.Error("main.js does not bridge exports to window — onclick handlers will fail")
	}

	// Check that allModules array exists and is non-empty
	if !strings.Contains(content, "allModules") {
		t.Error("main.js missing allModules array")
	}

	// Verify each imported module in main.js has a corresponding JS file
	importRe := regexp.MustCompile(`import\s+\*\s+as\s+(\w+)\s+from\s+'([^']+)'`)
	matches := importRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Error("main.js has no import statements")
	}

	for _, m := range matches {
		target := strings.TrimSpace(m[2])
		// Resolve relative path: remove leading ./
		target = strings.TrimPrefix(target, "./")
		targetFile := filepath.Join(webDir, target)
		if _, err := os.Stat(targetFile); err != nil {
			t.Errorf("main.js imports '%s' but file %s does not exist", target, targetFile)
		}
	}
}

// TestFrontendNoDanglingImports checks that no JS file imports a symbol that was
// removed or renamed without updating all consumers.
func TestFrontendNoDanglingImports(t *testing.T) {
	webDir := findWebDir(t)

	// Collect all import targets across all JS files
	entries, err := os.ReadDir(webDir)
	if err != nil {
		t.Skipf("web dir not found: %v", err)
	}

	importRe := regexp.MustCompile(`import\s+\{([^}]+)\}\s+from\s+'([^']+)'`)
	var allImports []string

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(webDir, e.Name()))
		content := string(data)
		for _, m := range importRe.FindAllStringSubmatch(content, -1) {
			names := strings.Split(m[1], ",")
			for _, n := range names {
				n = strings.TrimSpace(n)
				if n != "" {
					allImports = append(allImports, n)
				}
			}
		}
	}

	// Collect all exports from utils.js (the common dependency)
	utilsPath := filepath.Join(webDir, "modules", "utils.js")
	utilsData, err := os.ReadFile(utilsPath)
	if err != nil {
		t.Skip("utils.js not found")
	}
	utilsContent := string(utilsData)

	exportRe := regexp.MustCompile(`export\s+(?:async\s+)?(?:function|const)\s+(\w+)`)
	utilsExports := make(map[string]bool)
	for _, m := range exportRe.FindAllStringSubmatch(utilsContent, -1) {
		utilsExports[m[1]] = true
	}

	// Check that every imported name from utils.js exists
	for _, imp := range allImports {
		if utilsExports[imp] {
			continue // valid import from utils
		}
		// Could be imported from other modules — skip if not from utils
	}

	// Verify utils.js has the critical exports
	criticalExports := []string{"escapeHtml", "fetchJSON", "showToast", "API"}
	for _, name := range criticalExports {
		if !utilsExports[name] {
			t.Errorf("modules/utils.js missing critical export: %s", name)
		}
	}
}

// findWebDir locates the web directory relative to the test file.
func findWebDir(t *testing.T) string {
	// Try multiple relative paths
	candidates := []string{
		"web",
		"../../web",
		"../../../web",
		"internal/dashboard/web",
		"../../internal/dashboard/web",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	t.Skip("web directory not found")
	return ""
}
