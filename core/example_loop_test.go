package core_test

import (
	"context"
	"fmt"

	"github.com/resolute-sh/resolute/core"
)

// ExampleLoop demonstrates the Steps-based Loop API: each iteration runs the
// noop body Step, AfterIteration mutates typed state by incrementing a
// counter, and While exits when the counter reaches 2.
func ExampleLoop() {
	type tick struct{ N int }

	noop := func(_ context.Context, _ struct{}) (struct{}, error) {
		return struct{}{}, nil
	}
	noopNode := core.NewNode("noop", noop, struct{}{})

	step := core.Loop[tick](
		core.StateKey[tick]("ticker"),
		core.InitialState[tick](func(_ *core.FlowState) tick { return tick{N: 0} }),
		core.Steps[tick](core.AsStep(noopNode)),
		core.AfterIteration[tick](func(s tick, _ core.FlowStateReader) tick {
			s.N++
			return s
		}),
		core.While[tick](func(s tick) bool { return s.N < 2 }),
		core.MaxIterations[tick](10),
	)
	_ = step

	fmt.Println("loop step constructed")
	// Output: loop step constructed
}
