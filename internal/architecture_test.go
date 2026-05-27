package internal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	appRoot              = "air-cover/internal/app"
	domainRoot           = "air-cover/internal/domain"
	adaptersRoot         = "air-cover/internal/adapters"
	inboundAdaptersRoot  = "air-cover/internal/adapters/inbound"
	outboundAdaptersRoot = "air-cover/internal/adapters/outbound"
	httpAdapterRoot      = "air-cover/internal/adapters/inbound"
	sqliteAdapterRoot    = "air-cover/internal/adapters/outbound/sqlite"
	platformRoot         = "air-cover/internal/platform"
)

func TestHexagonalImportBoundaries(t *testing.T) {
	packages, err := listPackages(includeTests(false))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}

	for _, pkg := range packages {
		switch {
		case pkg.ImportPath == domainRoot:
			forbidImports(t, pkg,
				appRoot,
				adaptersRoot,
				platformRoot,
				"database/sql",
				"net/http",
			)
		case strings.HasPrefix(pkg.ImportPath, appRoot):
			forbidImports(t, pkg, adaptersRoot, platformRoot)
		case strings.HasPrefix(pkg.ImportPath, inboundAdaptersRoot):
			forbidImports(t, pkg, outboundAdaptersRoot)
		case strings.HasPrefix(pkg.ImportPath, outboundAdaptersRoot):
			forbidImports(t, pkg, inboundAdaptersRoot)
		}
	}
}

func TestHexagonalTestImportBoundaries(t *testing.T) {
	packages, err := listPackages(includeTests(true))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}

	for _, pkg := range packages {
		switch {
		case pkg.ImportPath == domainRoot:
			forbidImports(t, pkg, adaptersRoot)
		case strings.HasPrefix(pkg.ImportPath, appRoot):
			forbidImports(t, pkg, adaptersRoot)
		}
	}
}

func TestInboundAdaptersDoNotImportOutboundAdapters(t *testing.T) {
	assertFilesDoNotImport(t, "adapters/inbound", outboundAdaptersRoot, "outbound adapter")
}

func TestOutboundAdaptersDoNotImportInboundAdapters(t *testing.T) {
	assertFilesDoNotImport(t, "adapters/outbound", inboundAdaptersRoot, "inbound adapter")
}

func assertFilesDoNotImport(t *testing.T, root string, forbiddenPrefix string, forbiddenName string) {
	t.Helper()

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath := strings.Trim(imported.Path.Value, `"`)
			if importPath == forbiddenPrefix || strings.HasPrefix(importPath, forbiddenPrefix+"/") {
				t.Errorf("%s imports %s package %s", path, forbiddenName, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func TestDomainTypesDoNotCarrySerializationTags(t *testing.T) {
	if err := assertNoStructTags(t, "domain", "json:", "form:"); err != nil {
		t.Fatalf("walk domain: %v", err)
	}
}

func assertNoStructTags(t *testing.T, dir string, forbiddenSubstrings ...string) error {
	t.Helper()

	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			field, ok := node.(*ast.Field)
			if !ok || field.Tag == nil {
				return true
			}
			tag := strings.Trim(field.Tag.Value, "`")
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(tag, forbidden) {
					t.Errorf("%s contains forbidden struct tag %q", path, tag)
					break
				}
			}
			return true
		})
		return nil
	})
}

func TestGeneratedSQLiteTypesStayInsideSQLiteAdapter(t *testing.T) {
	packages, err := listPackages(includeTests(false))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	for _, pkg := range packages {
		if strings.HasPrefix(pkg.ImportPath, sqliteAdapterRoot) {
			continue
		}
		forbidImports(t, pkg, sqliteAdapterRoot+"/dbgen")
	}
}

func TestHTTPPresentationPackagesStayHTTPOwned(t *testing.T) {
	packages, err := listPackages(includeTests(false))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	for _, pkg := range packages {
		if pkg.ImportPath == httpAdapterRoot || strings.HasPrefix(pkg.ImportPath, httpAdapterRoot+"/") {
			continue
		}
		forbidImports(t, pkg, httpAdapterRoot+"/ui", httpAdapterRoot+"/presenter")
	}
}

func TestConcreteAdaptersOnlyComposedByCmd(t *testing.T) {
	packages, err := listPackages(includeTests(false))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	for _, pkg := range packages {
		if pkg.ImportPath == "air-cover/internal/cmd" || strings.HasPrefix(pkg.ImportPath, adaptersRoot) {
			continue
		}
		forbidImports(t, pkg, adaptersRoot)
	}
}

func TestLegacyDeliveryPackagesStayRemoved(t *testing.T) {
	packages, err := listPackages(includeTests(true))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	for _, pkg := range packages {
		for _, legacy := range []string{"air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter"} {
			if pkg.ImportPath == legacy || strings.HasPrefix(pkg.ImportPath, legacy+"/") {
				t.Errorf("legacy delivery package %s should live under internal/adapters/inbound", pkg.ImportPath)
			}
		}
	}
}

func TestAppTypesDoNotCarryTransportTags(t *testing.T) {
	for _, dir := range []string{"domain", "app"} {
		if err := assertNoStructTags(t, dir, "json:", "form:"); err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

type listedPackage struct {
	ImportPath string
	Imports    []string
}

type packageListOption func(*packageListConfig)

type packageListConfig struct {
	includeTests bool
}

func includeTests(include bool) packageListOption {
	return func(config *packageListConfig) {
		config.includeTests = include
	}
}

func listPackages(options ...packageListOption) ([]listedPackage, error) {
	config := packageListConfig{}
	for _, option := range options {
		option(&config)
	}

	packageImports := map[string]map[string]bool{}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !config.includeTests && strings.HasSuffix(path, "_test.go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}

		importPath := "air-cover/internal"
		if dir := filepath.Dir(path); dir != "." {
			importPath += "/" + filepath.ToSlash(dir)
		}
		if packageImports[importPath] == nil {
			packageImports[importPath] = map[string]bool{}
		}
		for _, imported := range parsed.Imports {
			packageImports[importPath][strings.Trim(imported.Path.Value, `"`)] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	listed := make([]listedPackage, 0, len(packageImports))
	for importPath, imports := range packageImports {
		pkg := listedPackage{ImportPath: importPath}
		for imported := range imports {
			pkg.Imports = append(pkg.Imports, imported)
		}
		sort.Strings(pkg.Imports)
		listed = append(listed, pkg)
	}
	sort.Slice(listed, func(i, j int) bool {
		return listed[i].ImportPath < listed[j].ImportPath
	})
	return listed, nil
}

func forbidImports(t *testing.T, pkg listedPackage, forbiddenPrefixes ...string) {
	t.Helper()

	for _, imported := range pkg.Imports {
		for _, forbidden := range forbiddenPrefixes {
			if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
				t.Errorf("%s imports forbidden package %s", pkg.ImportPath, imported)
			}
		}
	}
}
