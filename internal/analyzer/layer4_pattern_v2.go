// layer4_pattern_v2.go adds TypeScript-specific pattern detection to the L4 layer.
package analyzer

import (
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// DetectTS runs all TypeScript-specific pattern detectors and appends results
// to the provided PatternResult.
func (d *PatternDetector) DetectTS(astInfo *types.ASTInfo, chains []*types.CallChain, result *types.PatternResult) {
	d.detectPageObjectTS(astInfo, chains, result)
	d.detectSetupTeardownTS(astInfo, chains, result)
	d.detectChainCallTS(chains, result)
	d.detectAsyncAwaitTS(chains, result)
	d.detectDescribeBlockTS(astInfo, result)
}

func (d *PatternDetector) detectPageObjectTS(astInfo *types.ASTInfo, chains []*types.CallChain, result *types.PatternResult) {
	for _, imp := range astInfo.Imports {
		if strings.HasSuffix(imp.Path, ".page") || strings.Contains(imp.Path, "/pages/") {
			addPattern(result, types.PatternPageObjectTS,
				"将 TypeScript Page Object 类转换为 midscene Page 对象，方法内的 nep 调用替换为 ai* 意图调用")
			return
		}
	}
}

func (d *PatternDetector) detectSetupTeardownTS(astInfo *types.ASTInfo, chains []*types.CallChain, result *types.PatternResult) {
	for _, fn := range astInfo.Functions {
		name := fn.Name
		if name == "beforeEach" || name == "afterEach" ||
			name == "beforeAll" || name == "afterAll" {
			addPattern(result, types.PatternSetupTeardown,
				"将 beforeEach/afterEach 迁移至 midscene 的 BeforeAll/AfterAll 钩子")
			return
		}
	}

	for _, chain := range chains {
		for _, step := range chain.Steps {
			fn := step.FuncName
			if fn == "beforeEach" || fn == "afterEach" ||
				fn == "beforeAll" || fn == "afterAll" {
				addPattern(result, types.PatternSetupTeardown,
					"将 beforeEach/afterEach 迁移至 midscene 的 BeforeAll/AfterAll 钩子")
				return
			}
		}
	}
}

func (d *PatternDetector) detectChainCallTS(chains []*types.CallChain, result *types.PatternResult) {
	const minChainLength = 3

	for _, chain := range chains {
		if len(chain.Steps) < minChainLength {
			continue
		}

		runLen := 1
		for i := 1; i < len(chain.Steps); i++ {
			prev := chain.Steps[i-1]
			curr := chain.Steps[i]

			sameReceiver := curr.Receiver != "" && curr.Receiver == prev.Receiver
			bothAwait := curr.IsAwait && prev.IsAwait

			if sameReceiver && bothAwait {
				runLen++
				if runLen >= minChainLength {
					addPattern(result, types.PatternChainCall,
						"连续 await 链式调用可合并为 midscene 的单条 ai 意图描述")
					return
				}
			} else {
				runLen = 1
			}
		}
	}
}

func (d *PatternDetector) detectAsyncAwaitTS(chains []*types.CallChain, result *types.PatternResult) {
	awaitCount := 0
	for _, chain := range chains {
		for _, step := range chain.Steps {
			if step.IsAwait {
				awaitCount++
			}
		}
	}

	totalCalls := 0
	for _, chain := range chains {
		totalCalls += len(chain.Steps)
	}

	if totalCalls > 0 && awaitCount > totalCalls/2 {
		_ = awaitCount
	}
}

func (d *PatternDetector) detectDescribeBlockTS(astInfo *types.ASTInfo, result *types.PatternResult) {
	for _, fn := range astInfo.Functions {
		name := fn.Name
		if name == "describe" || name == "it" || name == "test" {
			return
		}
	}
}
