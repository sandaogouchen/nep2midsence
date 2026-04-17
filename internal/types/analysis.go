package types

// FullAnalysis is the complete analysis result for a single case file
type FullAnalysis struct {
	FilePath     string           `json:"file_path"`
	TargetPath   string           `json:"target_path"`
	Package      string           `json:"package"`
	Complexity   string           `json:"complexity"` // simple / medium / complex
	Language     string           `json:"language,omitempty"`

	AST          *ASTInfo         `json:"ast"`
	CallChains   []*CallChain     `json:"call_chains"`
	DataFlows    []*ValueInfo     `json:"data_flows"`
	Patterns     *PatternResult   `json:"patterns"`
	Intents      []*IntentInfo    `json:"intents"`
	APIMappings  []*AnnotatedCall `json:"api_mappings"`
	Dependencies []string         `json:"dependencies"`
}

// ASTInfo holds L1 AST structural analysis results
type ASTInfo struct {
	FilePath   string       `json:"file_path"`
	Package    string       `json:"package"`
	Imports    []ImportInfo `json:"imports"`
	Functions  []FuncInfo   `json:"functions"`
	Structs    []StructInfo `json:"structs"`
	Constants  []ConstInfo  `json:"constants"`
	Variables  []VarInfo    `json:"variables"`
	InitBlocks []InitInfo   `json:"init_blocks"`
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
