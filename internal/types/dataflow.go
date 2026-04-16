package types

type ValueKind string

const (
	ValueLiteral       ValueKind = "literal"
	ValueConst         ValueKind = "const"
	ValueConfig        ValueKind = "config"
	ValueFuncReturn    ValueKind = "func_return"
	ValueParam         ValueKind = "param"
	ValueConcatenation ValueKind = "concatenation"
	ValueUnresolved    ValueKind = "unresolved"
)

type ValueInfo struct {
	Kind       ValueKind   `json:"kind"`
	Value      string      `json:"value"`
	Source     string      `json:"source"`
	DefinedAt  Location    `json:"defined_at"`
	UsedBy     []Usage     `json:"used_by"`
	Components []ValueInfo `json:"components,omitempty"`
	Variable   string      `json:"variable"`
}

type Location struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type Usage struct {
	NepAPI   string   `json:"nep_api"`
	ArgIndex int      `json:"arg_index"`
	Location Location `json:"location"`
}
