package core_test

import (
	"fmt"

	"github.com/resolute-sh/resolute/core"
)

// ExampleLoop demonstrates constructing a Loop step that threads a typed
// counter state across iterations and exits when the While predicate
// returns false.
//
// The Body option is omitted here for brevity; production callers pass a
// real LoopBody[S] whose Execute typically calls workflow.ExecuteActivity
// to invoke a registered activity. InitialState reads the seed S from
// *FlowState; FinalState writes the final S back. While, MaxIterations,
// InitialState, and FinalState all run in workflow code and must be
// deterministic.
func ExampleLoop() {
	type counter struct{ N int }

	step := core.Loop[counter](
		core.InitialState[counter](func(_ *core.FlowState) counter { return counter{} }),
		core.While[counter](func(s counter) bool { return s.N < 3 }),
		core.MaxIterations[counter](100),
		core.FinalState[counter](func(_ *core.FlowState, _ counter) {}),
	)
	_ = step

	fmt.Println("loop step constructed")
	// Output: loop step constructed
}
