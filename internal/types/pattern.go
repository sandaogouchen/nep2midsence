package types

type PatternType string

const (
	PatternPageObject    PatternType = "page_object"
	PatternChainCall     PatternType = "chain_call"
	PatternDataDriven    PatternType = "data_driven"
	PatternSetupTeardown PatternType = "setup_teardown"
	PatternExplicitWait  PatternType = "explicit_wait"
	PatternIframe        PatternType = "iframe"
	PatternMultiTab      PatternType = "multi_tab"
	PatternScreenshot    PatternType = "screenshot"
	PatternRetryLoop     PatternType = "retry_loop"
)

type PatternResult struct {
	Detected   []PatternType          `json:"detected"`
	Strategies map[PatternType]string `json:"strategies"`
	Complexity string                 `json:"complexity"`
}
