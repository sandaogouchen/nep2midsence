package types

type IntentInfo struct {
	FuncName    string       `json:"func_name"`
	FuncDoc     string       `json:"func_doc"`
	FuncIntent  string       `json:"func_intent"`
	StepIntents []StepIntent `json:"step_intents"`
}

type StepIntent struct {
	StepIndex      int     `json:"step_index"`
	NepAPICall     string  `json:"nep_api_call"`
	InlineComment  string  `json:"inline_comment"`
	InferredIntent string  `json:"inferred_intent"`
	IntentSource   string  `json:"intent_source"`
	Confidence     float64 `json:"confidence"`
}
