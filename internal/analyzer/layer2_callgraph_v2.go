// layer2_callgraph_v2.go adds TypeScript support to the L2 call graph layer.
//
// When the TS bridge has already extracted all call information from the
// TypeScript AST, BuildChainsFromTSCalls groups those flat calls into
// per-test-function CallChains without any Go AST walking.
package analyzer

import (
	"sort"

	"github.com/sandaogouchen/nep2midsence/internal/types"
)

// BuildChainsFromTSCalls creates call chains from pre-extracted TypeScript call
// data instead of parsing Go AST. This is used when the TS bridge has already
// extracted all call information.
//
// Algorithm:
//  1. Collect all test functions from astInfo (IsTest == true).
//  2. Sort calls by line number.
//  3. For each test function, collect calls whose line falls within the
//     function's [LineStart, LineEnd] range.
//  4. Build a CallChain with those calls as Steps, marking NepAPICalls.
//  5. Calls that fall outside any test function are grouped into an
//     "orphan" chain (e.g., top-level describe/it blocks with no
//     enclosing test function).
func (a *CallGraphAnalyzer) BuildChainsFromTSCalls(astInfo *types.ASTInfo, allCalls []types.CallStep) []*types.CallChain {
	// Sort calls by line for deterministic grouping.
	sorted := make([]types.CallStep, len(allCalls))
	copy(sorted, allCalls)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Line < sorted[j].Line
	})

	// Collect test functions sorted by start line.
	var testFuncs []types.FuncInfo
	for _, fn := range astInfo.Functions {
		if fn.IsTest {
			testFuncs = append(testFuncs, fn)
		}
	}
	sort.Slice(testFuncs, func(i, j int) bool {
		return testFuncs[i].LineStart < testFuncs[j].LineStart
	})

	// Track which calls are claimed by a test function.
	claimed := make([]bool, len(sorted))

	var chains []*types.CallChain

	for _, fn := range testFuncs {
		chain := &types.CallChain{
			TestFunc:  fn.Name,
			StartLine: fn.LineStart,
			EndLine:   fn.LineEnd,
		}

		for idx, call := range sorted {
			if call.Line >= fn.LineStart && call.Line <= fn.LineEnd {
				chain.Steps = append(chain.Steps, call)
				if call.IsNepAPI {
					chain.NepAPICalls = append(chain.NepAPICalls, call)
				}
				claimed[idx] = true
			}
		}

		if len(chain.Steps) > 0 {
			chains = append(chains, chain)
		}
	}

	// Collect orphan calls (outside any test function) into a single chain.
	var orphanSteps []types.CallStep
	var orphanNep []types.CallStep
	for idx, call := range sorted {
		if !claimed[idx] {
			orphanSteps = append(orphanSteps, call)
			if call.IsNepAPI {
				orphanNep = append(orphanNep, call)
			}
		}
	}

	if len(orphanSteps) > 0 {
		orphanChain := &types.CallChain{
			TestFunc:    "<top-level>",
			Steps:       orphanSteps,
			NepAPICalls: orphanNep,
		}
		if len(orphanSteps) > 0 {
			orphanChain.StartLine = orphanSteps[0].Line
			orphanChain.EndLine = orphanSteps[len(orphanSteps)-1].Line
		}
		chains = append(chains, orphanChain)
	}

	return chains
}
