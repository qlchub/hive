# Hive

[![Go Reference](https://pkg.go.dev/badge/github.com/qlchub/hive.svg)](https://pkg.go.dev/github.com/qlchub/hive)
[![CI/CD](https://github.com/qlchub/hive/actions/workflows/ci.yaml/badge.svg)](https://github.com/qlchub/hive/actions/workflows/ci.yaml)

Hive is a lightweight, production-ready actor model implementation for Go. It provides a robust framework for building concurrent, stateful, and resilient systems by encapsulating state and behavior within independent actors. The name "Hive" conveys the concept of a collection of busy, autonomous workers collaborating to achieve a common goal.

## Features

-   **Simple and Looping Actors**: Supports both simple request/response actors and long-running looping actors for complex workflows.
-   **Automatic Panic Recovery**: Simple actors are automatically restarted if they panic, ensuring system resilience.
-   **Backpressure Handling**: Non-blocking message dispatch prevents a slow actor from causing cascading failures. Callers are notified immediately if an actor's mailbox is full.
-   **Configurable Mailboxes**: Tune actor performance by specifying mailbox sizes on a per-type basis.
-   **Graceful Shutdown**: Coordinated shutdown of all actors via `context.Context`.
-   **Structured Logging**: Integrates with `log/slog` for structured, context-aware logging.
-   **Type-Safe Request/Response**: Generic helpers for building type-safe request/response patterns.

## Installation

```sh
go get github.com/qlchub/hive
```

## Core Concepts

-   **Actor**: The fundamental unit of computation. An `Actor` has a `HandleMessage` method for processing incoming messages.
-   **LoopingActor**: A specialized actor that manages its own message-processing loop. Ideal for actors that need to manage timers, tickers, or interact with external I/O.
-   **Registry**: Manages the lifecycle of all actors. It's responsible for registering actor types, creating actor instances, and dispatching messages.
-   **ID**: A unique identifier for an actor instance, composed of a `Type` (e.g., "user") and an `EntityID` (e.g., "user-123").
-   **Message**: The unit of communication between actors. All messages must implement the `hive.Message` interface.

## Usage

### 1. Defining a Simple Actor

A simple `Actor` processes one message at a time. State is encapsulated within the actor's struct.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/qlchub/hive"
)

// GreeterMessage is the message our actor will handle.
type GreeterMessage struct {
	*hive.BaseMessage
	Name string
}

// GreeterActor holds the state and logic.
type GreeterActor struct{}

// NewGreeterActor is the producer function.
func NewGreeterActor() hive.Actor {
	return &GreeterActor{}
}

// HandleMessage processes the message.
func (a *GreeterActor) HandleMessage(ctx *hive.Context, msg hive.Message) error {
	greeterMsg, ok := msg.(*GreeterMessage)
	if !ok {
		return fmt.Errorf("unexpected message type: %T", msg)
	}
	ctx.Logger().Info(fmt.Sprintf("Hello, %s!", greeterMsg.Name))
	return nil
}

func main() {
	// 1. Create a registry.
	registry := hive.NewRegistry(context.Background())
	defer registry.GracefulStop()
	registry.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	// 2. Register the actor type with a producer function.
	const greeterActorType hive.Type = "greeter"
	registry.MustRegisterActor(greeterActorType, NewGreeterActor)

	// 3. Get an ID and dispatch a message.
	// The actor is created automatically on the first message.
	actorID := hive.NewID(greeterActorType, "my-greeter")
	msg := &GreeterMessage{BaseMessage: hive.NewBaseMessage(), Name: "Hive"}
	if err := registry.DispatchMessage(actorID, msg); err != nil {
		fmt.Printf("Failed to dispatch: %v\n", err)
	}

	time.Sleep(100 * time.Millisecond)
}
```

### 2. Defining a Looping Actor

A `LoopingActor` runs its own `for/select` loop, giving it full control over message processing, timers, and other events.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/qlchub/hive"
)

// TickerActor is the factory for our looper.
type TickerActor struct{}

func NewTickerActor() hive.LoopingActor { return &TickerActor{} }

// NewActorLoop creates the looper instance.
func (a *TickerActor) NewActorLoop(ctx *hive.Context) hive.ActorLooper {
	return &TickerActorLooper{
		ctx:    ctx,
		ticker: time.NewTicker(1 * time.Second),
	}
}

// TickerActorLooper contains the state and loop logic.
type TickerActorLooper struct {
	ctx    *hive.Context
	ticker *time.Ticker
}

// ActorLoop is the main execution loop.
func (l *TickerActorLooper) ActorLoop(msgCh <-chan hive.Message) error {
	defer l.ticker.Stop()
	l.ctx.Logger().Info("Ticker loop started")

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				return nil // Channel closed, shutdown.
			}
			l.ctx.Logger().Info("Received message", "type", fmt.Sprintf("%T", msg))
		case t := <-l.ticker.C:
			l.ctx.Logger().Info("Tick", "time", t)
		case <-l.ctx.ProcessCtx().Done():
			return nil // Shutdown signal.
		}
	}
}

func main() {
	registry := hive.NewRegistry(context.Background())
	defer registry.GracefulStop()
	registry.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	const tickerActorType hive.Type = "ticker"
	registry.MustRegisterLoopingActor(tickerActorType, NewTickerActor)

	// Start the actor without an initial message.
	actorID, err := registry.RunActor(tickerActorType)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Started ticker actor: %s\n", actorID)

	time.Sleep(3 * time.Second)
}
```

## Production-Ready Features

Hive is designed with production requirements in mind.

### Panic Recovery

If a simple `Actor` panics while handling a message, its process will automatically recover. It logs the panic, discards the faulty actor instance, and creates a new one using the original producer function. This prevents a single buggy actor from being permanently disabled.

### Backpressure and Non-Blocking Dispatch

`DispatchMessage` is non-blocking. If an actor's mailbox is full, the call returns an error immediately: `hive.process.sendMessage: actor mailbox is full`.

This design prevents a slow actor from blocking the caller and causing cascading failures. The caller can then decide how to handle the backpressure, for example, by implementing a retry strategy with backoff:

```go
func dispatchWithRetry(dispatcher hive.Dispatcher, actorID hive.ID, msg hive.Message, retries int, backoff time.Duration) error {
	var err error
	for i := 0; i < retries; i++ {
		err = dispatcher.DispatchMessage(actorID, msg)
		if err == nil {
			return nil // Success
		}

		// Check for a full mailbox error.
		if strings.Contains(err.Error(), "actor mailbox is full") {
			time.Sleep(backoff)
			continue // Retry
		}

		// For other errors, fail immediately.
		return err
	}
	return fmt.Errorf("failed after %d retries: %w", retries, err)
}
```

### Configurable Mailbox Size

The default mailbox size for an actor is 64. You can override this on a per-type basis using the `WithMailboxSize` option during registration. This allows you to tune actors based on their expected message volume and processing speed.

```go
// Register an actor with a larger mailbox.
registry.MustRegisterActor(
    "high-volume-actor",
    NewHighVolumeActor,
    hive.WithMailboxSize(256),
)
```

## Performance

Hive is designed for high-throughput, low-latency workloads. Benchmarks demonstrate that it scales efficiently when work is distributed
across multiple actors.

### Benchmark Results

The following benchmarks were run on an AMD Ryzen 5 7640U CPU. They measure the performance of `DispatchMessage` under different levels
of concurrency and actor distribution.


```sh
goos: linux goarch: amd64 pkg: github.com/qlchub/hive cpu: AMD Ryzen 5 7640U w/ Radeon 760M Graphics
BenchmarkDispatch/1-Goroutine_1-Actor-12                 1000000              1052 ns/op             464 B/op         12 allocs/op
BenchmarkDispatch/10-Goroutines_1-Actor-12               1000000              1596 ns/op             770 B/op         20 allocs/op
BenchmarkDispatch/10-Goroutines_100-Actors-12           46053830                39.04 ns/op            5 B/op          0 allocs/op
BenchmarkDispatch/100-Goroutines_100-Actors-12          36302100                51.28 ns/op           11 B/op          0 allocs/op
BenchmarkDispatch/100-Goroutines_1000-Actors-12         33734709                35.04 ns/op            0 B/op          0 allocs/op
```


### Key Takeaways for Users

-   **Exceptional Performance for Distributed Workloads**: When your application's work is spread across many actors (e.g., one actor per
user session, device, or job), Hive achieves extremely high throughput (**35-50 ns/op**) with virtually **zero memory allocations**. This
makes it ideal for scalable, concurrent systems.

-   **Architectural Guidance**: The performance data highlights a key design principle of actor systems: **avoid bottlenecks**. Funneling
high-volume, concurrent requests to a single actor will lead to contention, as seen in the single-actor benchmarks. For optimal
performance, partition your work across a larger number of actors.

-   **Proven Scalability**: The framework scales efficiently as you add more actors and goroutines, ensuring your application can handle
growing loads without a significant increase in per-message overhead.

## Development

This project uses Go modules and includes a `Makefile` for common development tasks.

### Prerequisites

-   Go (version 1.25 or later)
-   `mockery` for generating mocks: `go install github.com/vektra/mockery/v3@latest`

### Running Tests

To run all tests and generate mocks:

```sh
make test
```

This command will first run `go tool mockery` to generate mocks based on `.mockery.yaml` and then execute `go test`.

## License

This project is licensed under the MIT License.
