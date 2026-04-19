package types

// FullAnalysis is the complete analysis result for a single case file
type FullAnalysis struct {
	FilePath   string `json:"file_path"`
	TargetPath string `json:"target_path"`
	TaskKey    string `json:"task_key,omitempty"`
	TaskKind   string `json:"task_kind,omitempty"` // case / helper
	Package    string `json:"package"`
	Complexity string `json:"complexity"` // simple / medium / complex
	Language   string `json:"language,omitempty"`

	AST          *ASTInfo         `json:"ast"`
	CallChains   []*CallChain     `json:"call_chains"`
	DataFlows    []*ValueInfo     `json:"data_flows"`
	Patterns     *PatternResult   `json:"patterns"`
	Intents      []*IntentInfo    `json:"intents"`
	APIMappings  []*AnnotatedCall `json:"api_mappings"`
	Dependencies []string         `json:"dependencies"`

	// DefaultPrompts collects component-level DEFAULT_PROMPT strings (NEP-specific
	// AI element descriptions) discovered from dependent files.
	DefaultPrompts []DefaultPromptInfo `json:"default_prompts,omitempty"`

	// HelperPlan describes a minimal helper/module migration task synthesized from
	// case wrapper calls. It is only populated for helper tasks.
	HelperPlan *HelperMigrationPlan `json:"helper_plan,omitempty"`

	// UnresolvedHelpers lists wrapper dependencies that could not be resolved to
	// a minimal helper migration task. Case prompts use this to preserve the
	// original call and add TODO annotations instead of forcing whole-file helper
	// migration.
	UnresolvedHelpers []UnresolvedHelper `json:"unresolved_helpers,omitempty"`

	// ResolvedSymbolDeps records symbol-level local-import resolution, including
	// barrel re-export tracing and shared-dependency preferences.
	ResolvedSymbolDeps []ResolvedSymbolDependency `json:"resolved_symbol_deps,omitempty"`

	// WrapperInjectedParams lists parameter names injected by the wrapper (e.g., commonIt).
	// These are fake dependencies that should NOT be migrated as real imports.
	WrapperInjectedParams []string `json:"wrapper_injected_params,omitempty"`
}

// ASTInfo holds L1 AST structural analysis results
type ASTInfo struct {
	FilePath      string       `json:"file_path"`
	Package       string       `json:"package"`
	Imports       []ImportInfo `json:"imports"`
	Functions     []FuncInfo   `json:"functions"`
	Structs       []StructInfo `json:"structs"`
	Constants     []ConstInfo  `json:"constants"`
	Variables     []VarInfo    `json:"variables"`
	InitBlocks    []InitInfo   `json:"init_blocks"`
	ClassName     string       `json:"class_name,omitempty"`
	ExtendsFrom   string       `json:"extends_from,omitempty"`
	ExtendsImport string       `json:"extends_import,omitempty"`
}

type ImportInfo struct {
	Path   string   `json:"path"`
	Alias  string   `json:"alias"`
	IsNep  bool     `json:"is_nep"`
	UsedBy []string `json:"used_by"`

	// TS-specific fields used by ts_bridge.go
	Name string `json:"name,omitempty"`
	Line int    `json:"line,omitempty"`
}

type FuncInfo struct {
	Name      string      `json:"name"`
	Doc       string      `json:"doc"`
	Params    []ParamInfo `json:"params"`
	Results   []ParamInfo `json:"results"`
	IsTest    bool        `json:"is_test"`
	IsHelper  bool        `json:"is_helper"`
	SubTests  []string    `json:"sub_tests"`
	LineStart int         `json:"line_start"`
	LineEnd   int         `json:"line_end"`
	Body      string      `json:"body"`
	Receiver  string      `json:"receiver"`

	// Wrapper fields: populated when the test is wrapped by commonIt or similar
	WrapperName           string   `json:"wrapper_name,omitempty"`
	WrapperInjectedParams []string `json:"wrapper_injected_params,omitempty"`
	WrapperOptions        string   `json:"wrapper_options,omitempty"`
	WrapperUrl            string   `json:"wrapper_url,omitempty"`
}

type ParamInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type StructInfo struct {
	Name    string      `json:"name"`
	Fields  []FieldInfo `json:"fields"`
	Methods []string    `json:"methods"`
	Doc     string      `json:"doc"`
}

type FieldInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type ConstInfo struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type"`
	Line  int    `json:"line,omitempty"` // TS-specific, used by layer3_dataflow_v2.go
}

type VarInfo struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Line  int    `json:"line,omitempty"` // TS-specific, used by layer3_dataflow_v2.go
}

type InitInfo struct {
	Kind      string `json:"kind"` // "init" or "TestMain"
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Body      string `json:"body"`
}

// DefaultPromptInfo captures a component class's static DEFAULT_PROMPT value.
// This is used to guide Midscene AI intent generation during migration.
type DefaultPromptInfo struct {
	ClassName   string `json:"class_name"`
	PromptValue string `json:"prompt_value"`
	FilePath    string `json:"file_path"`
	Line        int    `json:"line,omitempty"`
}

type HelperMigrationPlan struct {
	Receiver       string   `json:"receiver"`
	PageObjectFile string   `json:"page_object_file,omitempty"`
	Methods        []string `json:"methods,omitempty"`
}

type ResolvedSymbolDependency struct {
	ImportPath        string `json:"import_path"`
	ImportSpec        string `json:"import_spec,omitempty"`
	ImportedName      string `json:"imported_name"`
	LocalAlias        string `json:"local_alias,omitempty"`
	BarrelFile        string `json:"barrel_file,omitempty"`
	ExportFile        string `json:"export_file,omitempty"`
	ExportName        string `json:"export_name,omitempty"`
	TargetFile        string `json:"target_file,omitempty"`
	DependencyKind    string `json:"dependency_kind,omitempty"`
	IsSharedPreferred bool   `json:"is_shared_preferred,omitempty"`
}

type UnresolvedHelper struct {
	Receiver         string `json:"receiver"`
	Method           string `json:"method"`
	Reason           string `json:"reason"`
	ReceiverReachable bool  `json:"receiver_reachable,omitempty"`
}
