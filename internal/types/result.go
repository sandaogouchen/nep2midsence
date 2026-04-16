package types

import "time"

type MigrationResult struct {
	CaseFile   string        `json:"case_file"`
	TargetFile string        `json:"target_file"`
	Success    bool          `json:"success"`
	Output     string        `json:"output"`
	Error      string        `json:"error,omitempty"`
	Duration   time.Duration `json:"duration"`
	RetryCount int           `json:"retry_count"`
}

type VerifyResult struct {
	CaseFile     string `json:"case_file"`
	CompileOK    bool   `json:"compile_ok"`
	CompileError string `json:"compile_error,omitempty"`
	TestOK       bool   `json:"test_ok"`
	TestError    string `json:"test_error,omitempty"`
	Diff         string `json:"diff,omitempty"`
}

type MigrationReport struct {
	TotalCases   int                        `json:"total_cases"`
	Succeeded    int                        `json:"succeeded"`
	Failed       int                        `json:"failed"`
	Skipped      int                        `json:"skipped"`
	SuccessRate  float64                    `json:"success_rate"`
	ByComplexity map[string]*ComplexityStats `json:"by_complexity"`
	FailedCases  []FailedCase               `json:"failed_cases"`
	Duration     time.Duration              `json:"duration"`
}

type ComplexityStats struct {
	Total     int     `json:"total"`
	Succeeded int     `json:"succeeded"`
	Failed    int     `json:"failed"`
	Rate      float64 `json:"rate"`
}

type FailedCase struct {
	File  string `json:"file"`
	Error string `json:"error"`
	Phase string `json:"phase"`
}

// MigrationRule defines a nep API to midscene API mapping
type MigrationRule struct {
	NepAPI             string    `json:"nep_api"`
	MidsceneEquivalent string    `json:"midscene_equivalent"`
	NeedsIntentRewrite bool      `json:"needs_intent_rewrite"`
	ArgTransform       string    `json:"arg_transform,omitempty"`
	Note               string    `json:"note,omitempty"`
	Examples           []Example `json:"examples,omitempty"`
}

type Example struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// CocoOutput represents the output from a Coco CLI execution
type CocoOutput struct {
	Success   bool          `json:"success"`
	Output    string        `json:"output"`
	SessionID string        `json:"session_id"`
	ExitCode  int           `json:"exit_code"`
	CostUSD   float64       `json:"cost_usd,omitempty"`
	Duration  time.Duration `json:"duration"`
}
