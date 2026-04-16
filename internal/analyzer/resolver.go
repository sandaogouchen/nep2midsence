package analyzer

import (
	"go/build"
	"path/filepath"
)

// ModulePackageResolver resolves import paths to filesystem paths using
// the module path as a prefix. This complements the prefix-based
// PackageResolver defined in layer1_ast.go with a module-aware strategy.
type ModulePackageResolver struct {
	projectRoot string
	modulePath  string
}

func NewModulePackageResolver(projectRoot, modulePath string) *ModulePackageResolver {
	return &ModulePackageResolver{
		projectRoot: projectRoot,
		modulePath:  modulePath,
	}
}

// Resolve tries to resolve an import path to a filesystem directory
func (r *ModulePackageResolver) Resolve(importPath string) (string, error) {
	// Try as a relative path within the module
	if len(importPath) > len(r.modulePath) && importPath[:len(r.modulePath)] == r.modulePath {
		relPath := importPath[len(r.modulePath)+1:]
		return filepath.Join(r.projectRoot, relPath), nil
	}

	// Try using Go build context
	pkg, err := build.Import(importPath, r.projectRoot, build.FindOnly)
	if err != nil {
		return "", err
	}
	return pkg.Dir, nil
}

// IsLocalPackage checks if an import path is within the current module
func (r *ModulePackageResolver) IsLocalPackage(importPath string) bool {
	return len(importPath) >= len(r.modulePath) && importPath[:len(r.modulePath)] == r.modulePath
}
