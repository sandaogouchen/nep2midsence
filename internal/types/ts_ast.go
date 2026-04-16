package types

// ---------------------------------------------------------------------------
// TypeScript AST extractor output types
// ---------------------------------------------------------------------------
// These structs map directly to the JSON schema produced by the TS extraction
// script (scripts/dist/ts-ast-extractor.js). Every field uses the camelCase
// JSON tag that the extractor emits.

// TSFileAnalysis represents the JSON output from the TypeScript AST extractor.
type TSFileAnalysis struct {
	FilePath          string          `json:"filePath"`
	FileName          string          `json:"fileName"`
	Language          string          `json:"language"`
	Imports           []TSImport      `json:"imports"`
	TopLevelVariables []TSVariable    `json:"topLevelVariables"`
	Functions         []TSFunction    `json:"functions"`
	AllCalls          []TSCall        `json:"allCalls"`
	TestStructure     TSTestStructure `json:"testStructure"`
	RawLineCount      int             `json:"rawLineCount"`
	ParseErrors       []string        `json:"parseErrors"`
	Warnings          []string        `json:"warnings"`
	ExtractedAt       string          `json:"extractedAt"`
}

// TSImport represents a single import statement extracted from a TypeScript file.
type TSImport struct {
	Module    string   `json:"module"`
	Names     []string `json:"names"`
	IsDefault bool     `json:"isDefault"`
	IsType    bool     `json:"isType"`
	Line      int      `json:"line"`
}

// TSVariable represents a top-level variable or constant declaration.
type TSVariable struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"` // "const", "let", "var"
	TypeStr     string `json:"typeStr"`
	Initializer string `json:"initializer"`
	Line        int    `json:"line"`
	IsExported  bool   `json:"isExported"`
}

// TSFunction represents a function or method declaration extracted from TypeScript.
type TSFunction struct {
	Name       string    `json:"name"`
	IsAsync    bool      `json:"isAsync"`
	IsTest     bool      `json:"isTest"`
	IsHelper   bool      `json:"isHelper"`
	Params     []TSParam `json:"params"`
	ReturnType string    `json:"returnType"`
	BodyText   string    `json:"bodyText"`
	StartLine  int       `json:"startLine"`
	EndLine    int       `json:"endLine"`
	Doc        string    `json:"doc"`
	IsExported bool      `json:"isExported"`
}

// TSParam represents a single function parameter.
type TSParam struct {
	Name     string `json:"name"`
	TypeStr  string `json:"typeStr"`
	Optional bool   `json:"optional"`
	Default  string `json:"default"`
}

// TSCall represents a single function/method call site found during extraction.
type TSCall struct {
	Callee    string `json:"callee"`
	Args      int    `json:"args"`
	Line      int    `json:"line"`
	InFunc    string `json:"inFunc"`
	IsAwait   bool   `json:"isAwait"`
	IsChained bool   `json:"isChained"`
}

// ---------------------------------------------------------------------------
// Test structure types
// ---------------------------------------------------------------------------

// TSTestStructure represents the high-level test organisation of a test file,
// including nested describe blocks, individual test cases, and hooks.
type TSTestStructure struct {
	Framework      string            `json:"framework"` // e.g. "jest", "vitest", "mocha"
	DescribeBlocks []TSDescribeBlock `json:"describeBlocks"`
	TopLevelTests  []TSTestCase      `json:"topLevelTests"`
	Hooks          []TSHook          `json:"hooks"`
}

// TSDescribeBlock represents a describe/context block that groups tests.
type TSDescribeBlock struct {
	Name      string            `json:"name"`
	Tests     []TSTestCase      `json:"tests"`
	Nested    []TSDescribeBlock `json:"nested"`
	Hooks     []TSHook          `json:"hooks"`
	StartLine int               `json:"startLine"`
	EndLine   int               `json:"endLine"`
}

// TSTestCase represents a single test (it/test) case.
type TSTestCase struct {
	Name      string `json:"name"`
	IsSkipped bool   `json:"isSkipped"`
	IsOnly    bool   `json:"isOnly"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// TSHook represents a lifecycle hook (beforeAll, afterEach, etc.).
type TSHook struct {
	Kind      string `json:"kind"` // "beforeAll", "beforeEach", "afterAll", "afterEach"
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
}

// ---------------------------------------------------------------------------
// Extended wrapper that carries TS-specific data alongside the unified ASTInfo.
// ---------------------------------------------------------------------------

// TSExtendedASTInfo wraps ASTInfo with TS-specific fields that do not exist in
// the base Go-centric ASTInfo schema. Use this when you need to retain the full
// fidelity of the TypeScript extraction (test structure, call sites, language tag).
type TSExtendedASTInfo struct {
	*ASTInfo
	TSTestStructure *TSTestStructure `json:"ts_test_structure,omitempty"`
	TSAllCalls      []TSCall         `json:"ts_all_calls,omitempty"`
	Language        string           `json:"language,omitempty"`
}
