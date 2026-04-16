package types

// CallChain represents the expanded call sequence from an entry function
type CallChain struct {
	EntryFunc   string     `json:"entry_func"`
	Steps       []CallStep `json:"steps"`
	MaxDepth    int        `json:"max_depth"`
	NepAPICalls []CallStep `json:"nep_api_calls"`
}

type CallStep struct {
	Depth          int            `json:"depth"`
	Caller         string         `json:"caller"`
	Callee         string         `json:"callee"`
	FilePath       string         `json:"file_path"`
	Line           int            `json:"line"`
	Args           []string       `json:"args"`
	ArgsRaw        []string       `json:"args_raw"`
	IsNepAPI       bool           `json:"is_nep_api"`
	IsLocal        bool           `json:"is_local"`
	ReturnType     string         `json:"return_type"`
	Comment        string         `json:"comment"`
	MigrationRule  *MigrationRule `json:"migration_rule,omitempty"`
}

// AnnotatedCall is a call step annotated with migration rules from the fingerprint library
type AnnotatedCall struct {
	Step CallStep       `json:"step"`
	Rule *MigrationRule `json:"rule"`
}
