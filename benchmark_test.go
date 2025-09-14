package hive_test

import (
	"context"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/qlchub/hive"
)

// BenchmarkActor is a minimal actor for performance testing.
// Its HandleMessage method does nothing, ensuring we measure framework overhead, not work.
type BenchmarkActor struct{}

func (a *BenchmarkActor) HandleMessage(ctx *hive.Context, msg hive.Message) error {
	// For warmup messages, signal the waitgroup.
	if bm, ok := msg.(*BenchmarkMessage); ok {
		if bm.wg != nil {
			bm.wg.Done()
		}
	}
	return nil
}

// BenchmarkMessage is a minimal message for performance testing.
type BenchmarkMessage struct {
	*hive.BaseMessage
	wg *sync.WaitGroup
}

// BenchmarkDispatch measures the throughput of the DispatchMessage function
// under various conditions of concurrency and actor distribution.
func BenchmarkDispatch(b *testing.B) {
	// Define benchmark scenarios
	scenarios := []struct {
		name          string
		numGoroutines int
		numActors     int
	}{
		{name: "1-Goroutine_1-Actor", numGoroutines: 1, numActors: 1},
		{name: "10-Goroutines_1-Actor", numGoroutines: 10, numActors: 1},
		{name: "10-Goroutines_100-Actors", numGoroutines: 10, numActors: 100},
		{name: "100-Goroutines_100-Actors", numGoroutines: 100, numActors: 100},
		{name: "100-Goroutines_1000-Actors", numGoroutines: 100, numActors: 1000},
	}

	const benchmarkActorType hive.Type = "benchmark"
	msg := &BenchmarkMessage{BaseMessage: hive.NewBaseMessage()}

	for _, s := range scenarios {
		b.Run(s.name, func(b *testing.B) {
			// Setup for the specific scenario
			registry := hive.NewRegistry(context.Background())
			registry.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
			defer registry.GracefulStop()

			// Use a large mailbox to avoid backpressure, ensuring we measure dispatch speed.
			registry.MustRegisterActor(benchmarkActorType, func() hive.Actor { return &BenchmarkActor{} }, hive.WithMailboxSize(65536))

			// Pre-create actor IDs
			actorIDs := make([]hive.ID, s.numActors)
			for i := range s.numActors {
				actorIDs[i] = hive.NewID(benchmarkActorType, hive.EntityID(strconv.Itoa(i)))
			}

			// Ensure all actors are started before the benchmark timer starts
			var wg sync.WaitGroup
			wg.Add(s.numActors)
			warmupMsg := &BenchmarkMessage{BaseMessage: hive.NewBaseMessage(), wg: &wg}

			for _, id := range actorIDs {
				// Dispatch a dummy message to start each actor and wait for it to be processed.
				err := registry.DispatchMessage(id, warmupMsg)
				if err != nil {
					b.Fatalf("Failed to dispatch warmup message: %v", err)
				}
			}
			wg.Wait()

			b.SetParallelism(s.numGoroutines)
			b.ResetTimer()

			// Run the benchmark in parallel
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					// Distribute messages across actors
					actorID := actorIDs[i%s.numActors]
					err := registry.DispatchMessage(actorID, msg)
					if err != nil {
						// If the mailbox is full, it's a contention scenario.
						// We retry until the message can be dispatched.
						// This measures throughput under saturation.
						if strings.Contains(err.Error(), "mailbox is full") {
							for {
								runtime.Gosched() // Yield to allow the actor to process messages.
								err = registry.DispatchMessage(actorID, msg)
								if err == nil {
									break
								}
								if !strings.Contains(err.Error(), "mailbox is full") {
									b.Fatalf("DispatchMessage failed with unexpected error: %v", err)
								}
							}
						} else {
							b.Fatalf("DispatchMessage failed: %v", err)
						}
					}
					i++
				}
			})
		})
	}
}
