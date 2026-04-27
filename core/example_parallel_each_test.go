package core_test

import (
	"fmt"

	"github.com/resolute-sh/resolute/core"
)

// ExampleParallelEach demonstrates constructing a ParallelEach step that
// fans out one activity per element of a runtime-derived slice and folds
// the ordered results back into *FlowState via Merge.
//
// The ParallelBody option is omitted here for brevity; production callers
// pass a real ItemActivity[I, O] whose Execute typically calls
// workflow.ExecuteActivity to invoke a registered activity. From and Merge
// run in workflow code and must be deterministic over *FlowState.
func ExampleParallelEach() {
	step := core.ParallelEach[int, int](
		core.From[int, int](func(_ *core.FlowState) []int { return []int{1, 2, 3} }),
		core.Merge[int, int](func(_ *core.FlowState, _ []int) {}),
		core.MaxConcurrency[int, int](4),
	)
	_ = step

	fmt.Println("parallel-each step constructed")
	// Output: parallel-each step constructed
}
