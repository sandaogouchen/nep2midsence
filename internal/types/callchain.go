package types

// CallChain represents the expanded call sequence from an entry function
type CallChain struct {
	EntryFunc   string     `json:"entry_func"`
	Steps       []CallStep `json:"steps"`
	MaxDepth    int        `json:"max_depth"`
	NepAPICalls []CallStep `json:"nep_api_calls"`
	
	// WrapperCalls contains non-NEP wrapper/module/component method calls that
	// are relevant for migration (e.g., pageObject.module.vv_setStandardBtn()).
	WrapperCalls []CallStep `json:"wrapper_calls,omitempty"`

	// TS-specific fields used by layer2_callgraph_v2.go
	TestFunc  string `json:"test_func,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type CallStep struct {
	Depth         int            `json:"depth"`
	Caller        string         `json:"caller"`
	Callee        string         `json:"callee"`
	FilePath      string         `json:"file_path"`
	Line          int            `json:"line"`
	Args          []string       `json:"args"`
	ArgsRaw       []string       `json:"args_raw"`
	IsNepAPI      bool           `json:"is_nep_api"`
	IsLocal       bool           `json:"is_local"`
	ReturnType    string         `json:"return_type"`
	Comment       string         `json:"comment"`
	MigrationRule *MigrationRule `json:"migration_rule,omitempty"`

	// TS-specific fields used by ts_bridge.go and layer4_pattern_v2.go
	Receiver      string `json:"receiver,omitempty"`
	FullReceiver  string `json:"full_receiver,omitempty"`
	FuncName      string `json:"func_name,omitempty"`
	IsNep         bool   `json:"is_nep,omitempty"`
	IsAwait       bool   `json:"is_await,omitempty"`
	IsChained     bool   `json:"is_chained,omitempty"`
	IsWrapperCall bool   `json:"is_wrapper_call,omitempty"`
	InFunc        string `json:"in_func,omitempty"`
}

// AnnotatedCall is a call step annotated with migration rules from the fingerprint library
type AnnotatedCall struct {
	Step CallStep       `json:"step"`
	Rule *MigrationRule `json:"rule"`
}
