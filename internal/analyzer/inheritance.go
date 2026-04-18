package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

type InheritanceGraph struct {
	ClassToFile      map[string]string
	ChildToParent    map[string]string
	FileToParentFile map[string]string
	FileToClass      map[string]string
	FileNepDirect    map[string]bool
	FileNepInherited map[string]bool
	tscp             *config.TsConfigPaths
}

type InheritanceChain struct {
	ClassName   string
	Chain       []string
	NepAncestor string
	Depth       int
}

func BuildInheritanceGraph(projectRoot string, tscp *config.TsConfigPaths) *InheritanceGraph {
	graph := &InheritanceGraph{
		ClassToFile:      make(map[string]string),
		ChildToParent:    make(map[string]string),
		FileToParentFile: make(map[string]string),
		FileToClass:      make(map[string]string),
		FileNepDirect:    make(map[string]bool),
		FileNepInherited: make(map[string]bool),
		tscp:             tscp,
	}

	fileAST := make(map[string]fileInheritanceInfo)
	_ = filepath.WalkDir(projectRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !isTypeScriptLikeFile(path) {
			return nil
		}

		ast, _, _, parseErr := extractTypeScriptFallback(path, config.DefaultConfig())
		if parseErr != nil || ast == nil {
			return nil
		}
		cleanPath := filepath.Clean(path)
		fileAST[cleanPath] = fileInheritanceInfo{
			ClassName:     ast.ClassName,
			ExtendsFrom:   ast.ExtendsFrom,
			ExtendsImport: ast.ExtendsImport,
		}
		if ast.ClassName != "" {
			graph.ClassToFile[ast.ClassName] = cleanPath
			graph.FileToClass[cleanPath] = ast.ClassName
		}
		if b, readErr := os.ReadFile(path); readErr == nil {
			graph.FileNepDirect[cleanPath] = hasDirectNepMarkers(string(b))
		}
		return nil
	})

	for filePath, info := range fileAST {
		if info.ClassName == "" || info.ExtendsFrom == "" {
			continue
		}
		graph.ChildToParent[info.ClassName] = info.ExtendsFrom
		parentFile := graph.ClassToFile[info.ExtendsFrom]
		if parentFile == "" {
			parentFile = resolveInheritanceImport(filePath, info.ExtendsImport, graph.tscp)
		}
		if parentFile != "" {
			graph.FileToParentFile[filePath] = parentFile
		}
	}

	for filePath := range fileAST {
		if graph.FileNepDirect[filePath] {
			continue
		}
		if graph.resolveNepAncestor(filePath, 10) != "" {
			graph.FileNepInherited[filePath] = true
		}
	}

	return graph
}

func (g *InheritanceGraph) IsNepRelated(filePath string) bool {
	if g == nil {
		return false
	}
	filePath = filepath.Clean(filePath)
	if g.FileNepDirect[filePath] {
		return true
	}
	if g.FileNepInherited[filePath] {
		return true
	}
	return g.resolveNepAncestor(filePath, 10) != ""
}

func (g *InheritanceGraph) GetNepAncestorChain(filePath string) string {
	if g == nil {
		return ""
	}
	filePath = filepath.Clean(filePath)
	chain := g.resolveChainByFile(filePath, 10)
	if len(chain.Chain) == 0 || chain.NepAncestor == "" {
		return ""
	}
	return strings.Join(chain.Chain, " -> ") + " (NEP)"
}

type fileInheritanceInfo struct {
	ClassName     string
	ExtendsFrom   string
	ExtendsImport string
}

func (g *InheritanceGraph) resolveNepAncestor(filePath string, maxDepth int) string {
	visited := make(map[string]struct{})
	current := filepath.Clean(filePath)
	for depth := 0; depth < maxDepth && current != ""; depth++ {
		if _, ok := visited[current]; ok {
			return ""
		}
		visited[current] = struct{}{}
		if g.FileNepDirect[current] {
			return current
		}
		current = g.FileToParentFile[current]
	}
	return ""
}

func (g *InheritanceGraph) resolveChainByFile(filePath string, maxDepth int) InheritanceChain {
	var chain InheritanceChain
	visited := make(map[string]struct{})
	current := filepath.Clean(filePath)
	for depth := 0; depth < maxDepth && current != ""; depth++ {
		if _, ok := visited[current]; ok {
			break
		}
		visited[current] = struct{}{}
		className := g.FileToClass[current]
		if className == "" {
			className = filepath.Base(current)
		}
		if chain.ClassName == "" {
			chain.ClassName = className
		}
		chain.Chain = append(chain.Chain, className)
		chain.Depth = depth + 1
		if g.FileNepDirect[current] {
			chain.NepAncestor = className
			break
		}
		current = g.FileToParentFile[current]
	}
	return chain
}

func resolveInheritanceImport(baseFile, importPath string, tscp *config.TsConfigPaths) string {
	importPath = strings.TrimSpace(importPath)
	if importPath == "" {
		return ""
	}
	if strings.HasPrefix(importPath, ".") {
		return resolveLocalTSImport(baseFile, importPath)
	}
	if tscp != nil && tscp.CanResolve(importPath) {
		for _, candidate := range tscp.Resolve(importPath) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func resolveLocalTSImport(baseFile, importPath string) string {
	baseDir := filepath.Dir(baseFile)
	base := filepath.Clean(filepath.Join(baseDir, importPath))
	for _, candidate := range buildTypeScriptCandidates(base) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func buildTypeScriptCandidates(base string) []string {
	if filepath.Ext(base) != "" {
		return []string{filepath.Clean(base)}
	}
	exts := []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts"}
	candidates := make([]string, 0, len(exts)*2)
	for _, ext := range exts {
		candidates = append(candidates, filepath.Clean(base+ext))
	}
	for _, ext := range exts {
		candidates = append(candidates, filepath.Clean(filepath.Join(base, "index"+ext)))
	}
	return candidates
}

func hasDirectNepMarkers(text string) bool {
	markers := []string{"ai.action(", "ai?.action(", "ai.getElement(", "ai?.getElement(", "clickElementByVL("}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
