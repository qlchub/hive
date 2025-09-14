package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/qlchub/hive"
)

// TickerMessage is a sample message for our looping actor.
type TickerMessage struct {
	*hive.BaseMessage
	Content string
}

// TickerActorLooper holds the state and logic for the actor's main loop.
// For a looping actor, the state is typically managed within the looper instance,
// as it persists for the entire lifetime of the actor's run.
type TickerActorLooper struct {
	ctx    *hive.Context
	ticker *time.Ticker
}

// NewTickerActorLooper creates a new looper instance.
func NewTickerActorLooper(ctx *hive.Context) *TickerActorLooper {
	return &TickerActorLooper{
		ctx:    ctx,
		ticker: time.NewTicker(1 * time.Second), // Our actor will "tick" every second.
	}
}

// ActorLoop is the main execution loop for the TickerActor.
// This is where the actor's primary logic resides. It runs in its own goroutine.
func (l *TickerActorLooper) ActorLoop(msgCh <-chan hive.Message) error {
	l.ctx.Logger().Info("Ticker actor loop started")
	defer l.ticker.Stop()
	defer l.ctx.Logger().Info("Ticker actor loop stopped")

	// The handler is created once and reused for all incoming messages.

	// 1. Define the core message processing logic.
	actualHandler := func(msg hive.Message) error {
		if tickerMsg, ok := msg.(*TickerMessage); ok {
			l.ctx.Logger().Info("Received a message", "content", tickerMsg.Content)
		} else {
			l.ctx.Logger().Warn("Received unexpected message type")
		}
		return nil
	}

	// 2. Define a filter. For this example, we'll filter out messages with "ignore me" content.
	filter := func(msg hive.Message) bool {
		if tickerMsg, ok := msg.(*TickerMessage); ok {
			return tickerMsg.Content != "ignore me"
		}
		return true
	}

	// 3. Create a wrapped handler.
	// We use the looper's context logger and the filter we just defined.
	wrappedHandler := hive.CreateMessageHandlerWithFilter(actualHandler, l.ctx.Logger(), filter)

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				// The message channel has been closed, which means the actor process
				// is shutting down. We should exit the loop.
				return nil
			}
			// 4. Process the incoming message using the wrapped handler.
			// The wrapper will handle logging, timing, and filtering.
			if err := wrappedHandler(msg); err != nil {
				// The wrapper already logs the error, but we could add more handling here if needed.
				l.ctx.Logger().Error("Handler returned an error that requires special attention", "error", err)
			}

		case t := <-l.ticker.C:
			// Perform a periodic task. In this case, we just log a "tick".
			// This could be used for polling, heartbeats, or other scheduled work.
			l.ctx.Logger().Info("Tick", "time", t.Format(time.RFC3339))

		case <-l.ctx.ProcessCtx().Done():
			// The actor's specific context is done. This is a signal for this
			// individual actor to shut down.
			l.ctx.Logger().Info("Process context is done, shutting down ticker actor.")
			return nil
		}
	}
}

// TickerActor is the factory for our TickerActorLooper.
// It implements the hive.LoopingActor interface. Its only job is to create
// a new looper when the actor is started.
type TickerActor struct{}

// NewTickerActor creates a new TickerActor.
func NewTickerActor() hive.LoopingActor {
	return &TickerActor{}
}

// NewActorLoop is called by the Hive system to create the looper instance.
func (a *TickerActor) NewActorLoop(ctx *hive.Context) hive.ActorLooper {
	return NewTickerActorLooper(ctx)
}

// dispatchWithRetry demonstrates how a caller can implement a retry mechanism
// for dispatching messages, particularly when an actor's mailbox might be full.
func dispatchWithRetry(dispatcher hive.Dispatcher, actorID hive.ID, msg hive.Message, retries int, backoff time.Duration) error {
	var err error
	for i := 0; i < retries; i++ {
		err = dispatcher.DispatchMessage(actorID, msg)
		if err == nil {
			// Message sent successfully.
			return nil
		}

		// Check if the error is due to a full mailbox. This is a simple string check for demonstration.
		// In a production system, it would be better to use custom error types for more reliable checking.
		if strings.Contains(err.Error(), "actor mailbox is full") {
			fmt.Printf("Mailbox full for actor %s, retrying in %v...\n", actorID, backoff)
			time.Sleep(backoff)
			// Optional: Implement exponential backoff.
			// backoff *= 2
			continue
		}

		// For any other error (e.g., actor type not registered, process shutting down), fail immediately.
		return fmt.Errorf("dispatch failed with a non-retriable error: %w", err)
	}
	// If all retries fail, return the last error.
	return fmt.Errorf("failed to dispatch message after %d retries: %w", retries, err)
}

func main() {
	// 1. Create a new actor registry.
	registry := hive.NewRegistry(context.Background())
	defer registry.GracefulStop()

	// Configure a logger.
	registry.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// 2. Define a unique actor type.
	const tickerActorType hive.Type = "ticker"

	// 3. Register the LoopingActor type with the registry.
	registry.MustRegisterLoopingActor(tickerActorType, NewTickerActor, hive.WithMailboxSize(10))

	// 4. Start an instance of the TickerActor.
	// We use RunActor to start it without needing an initial message.
	// The actor will begin its loop and start ticking immediately.
	tickerActorID, err := registry.RunActor(tickerActorType)
	if err != nil {
		panic(fmt.Sprintf("Failed to run ticker actor: %v", err))
	}
	fmt.Printf("Started TickerActor with ID: %s\n", tickerActorID.String())

	// Let the actor run and tick for a few seconds to observe its behavior.
	time.Sleep(3 * time.Second)

	// We can also send messages to it while it's running.
	fmt.Println("Sending a message to the ticker actor...")
	msg := &TickerMessage{BaseMessage: hive.NewBaseMessage(), Content: "Hello from main!"}
	err = dispatchWithRetry(registry, tickerActorID, msg, 3, 50*time.Millisecond)
	if err != nil {
		fmt.Printf("Failed to dispatch message: %v\n", err)
	}

	// Wait a moment to see it processed.
	time.Sleep(50 * time.Millisecond)

	// Send a message that will be filtered out by the handler.
	fmt.Println("Sending a message that will be filtered...")
	filteredMsg := &TickerMessage{BaseMessage: hive.NewBaseMessage(), Content: "ignore me"}
	err = dispatchWithRetry(registry, tickerActorID, filteredMsg, 3, 50*time.Millisecond)
	if err != nil {
		fmt.Printf("Failed to dispatch filtered message: %v\n", err)
	}

	// Wait a bit more to observe the actor's behavior.
	time.Sleep(2 * time.Second)

	fmt.Println("Shutting down registry...")
	// The deferred GracefulStop will be called, signaling all actors to stop.
}
