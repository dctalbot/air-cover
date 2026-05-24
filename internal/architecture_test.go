package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHexagonalImportBoundaries(t *testing.T) {
	packages, err := listPackages(includeTests(false))
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}

	for _, pkg := range packages {
		switch {
		case pkg.ImportPath == "air-cover/internal/domain":
			forbidImports(t, pkg, "air-cover/internal/app", "air-cover/internal/adapters", "air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter")
		case strings.HasPrefix(pkg.ImportPath, "air-cover/internal/app"):
			forbidImports(t, pkg, "air-cover/internal/adapters", "air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter")
		case pkg.ImportPath == "air-cover/internal/policy":
			forbidImports(t, pkg, "air-cover/internal/app", "air-cover/internal/adapters", "air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter")
		case strings.HasPrefix(pkg.ImportPath, "air-cover/internal/adapters"):
			forbidImports(t, pkg, "air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter")
		case pkg.ImportPath == "air-cover/internal/presenter":
			forbidImports(t, pkg, "air-cover/internal/api", "air-cover/internal/adapters")
		case strings.HasPrefix(pkg.ImportPath, "air-cover/internal/ui"):
			forbidImports(t, pkg, "air-cover/internal/adapters", "air-cover/internal/api")
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
		case pkg.ImportPath == "air-cover/internal/domain":
			forbidImports(t, pkg, "air-cover/internal/api", "air-cover/internal/adapters", "air-cover/internal/ui", "air-cover/internal/presenter")
		case strings.HasPrefix(pkg.ImportPath, "air-cover/internal/app"):
			forbidImports(t, pkg, "air-cover/internal/api", "air-cover/internal/ui", "air-cover/internal/presenter")
		case pkg.ImportPath == "air-cover/internal/policy":
			forbidImports(t, pkg, "air-cover/internal/api", "air-cover/internal/adapters", "air-cover/internal/ui", "air-cover/internal/presenter")
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
		listed = append(listed, pkg)
	}
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
