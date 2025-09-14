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

// GreeterMessage represents a message sent to the GreeterActor.
// It embeds actor.BaseMessage to satisfy the actor.Message interface.
type GreeterMessage struct {
	*hive.BaseMessage
	Name string
}

// NewGreeterMessage creates a new GreeterMessage with the given name.
func NewGreeterMessage(name string) *GreeterMessage {
	return &GreeterMessage{
		BaseMessage: hive.NewBaseMessage(),
		Name:        name,
	}
}

// GreeterActor is a simple actor that implements the actor.Actor interface.
type GreeterActor struct {
	// The actor's internal state can be defined here.
	// For a simple greeter, we might not need much state.
}

// NewGreeterActor creates a new instance of GreeterActor.
func NewGreeterActor() hive.Actor {
	return &GreeterActor{}
}

// handleGreeterMessage contains the core logic for processing a message.
func (a *GreeterActor) handleGreeterMessage(ctx *hive.Context, msg hive.Message) error {
	// Type assert the message to our specific GreeterMessage type.
	greeterMsg, ok := msg.(*GreeterMessage)
	if !ok {
		// This check is technically redundant if the filter is comprehensive,
		// but it's good practice for robustness.
		ctx.Logger().Error("received unexpected message type", slog.Any("message", msg))
		return fmt.Errorf("unexpected message type: %T", msg)
	}

	ctx.Logger().Info(fmt.Sprintf("Hello, %s!", greeterMsg.Name))

	// In a real scenario, you might perform more complex business logic here,
	// interact with databases, call external services, or dispatch new messages
	// to other actors using ctx.Dispatcher().

	return nil
}

// HandleMessage processes an incoming message.
// This method is called by the actor system when a message is dispatched to this actor.
// It demonstrates wrapping the core logic with CreateMessageHandlerWithFilter.
func (a *GreeterActor) HandleMessage(ctx *hive.Context, msg hive.Message) error {
	// We can define a filter to decide which messages to process.
	// For this example, we'll filter out any messages not of type *GreeterMessage
	// and any messages addressed to "World".
	filter := func(m hive.Message) bool {
		greeterMsg, ok := m.(*GreeterMessage)
		if !ok {
			return false // Filter out messages of the wrong type.
		}
		if greeterMsg.Name == "World" {
			return false // Filter out messages to "World".
		}
		return true
	}

	// The actual handler needs to match the signature `func(msg Message) error`.
	// We create a closure here to capture the `ctx` and `a` (the actor instance)
	// so they can be passed to our core logic function.
	actualHandler := func(m hive.Message) error {
		return a.handleGreeterMessage(ctx, m)
	}

	// Create the wrapped handler, providing the core logic, the actor's logger, and the filter.
	wrappedHandler := hive.CreateMessageHandlerWithFilter(actualHandler, ctx.Logger(), filter)

	// Execute the wrapped handler.
	return wrappedHandler(msg)
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
	// The registry manages actor lifecycles and message dispatching.
	registry := hive.NewRegistry(context.Background())
	defer registry.GracefulStop() // Ensure all actors are gracefully stopped when main exits.

	// Configure a logger for the registry (optional, but good practice).
	registry.SetLogger(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// 2. Define a unique actor type for our GreeterActor.
	const greeterActorType hive.Type = "greeter"

	// 3. Register the GreeterActor type with the registry.
	// We provide a producer function that the registry will use to create new
	// instances of GreeterActor when needed.
	registry.MustRegisterActor(greeterActorType, NewGreeterActor, hive.WithMailboxSize(128))

	// 4. Create an actor ID for our specific greeter instance.
	// For a regular actor, we typically want a stable ID if we expect to send
	// multiple messages to the same instance.
	greeterActorID := hive.NewID(greeterActorType, "my-first-greeter")

	// 5. Dispatch messages to the greeter actor.
	// The first message to an actor ID will cause the registry to create and
	// start a new instance of that actor type.
	fmt.Println("Dispatching messages...")

	// This message will be filtered out by the filter in HandleMessage and will not be processed.
	err := dispatchWithRetry(registry, greeterActorID, NewGreeterMessage("World"), 3, 50*time.Millisecond)
	if err != nil {
		fmt.Printf("Failed to dispatch message 1: %v\n", err)
	}

	// This message will be processed.
	err = dispatchWithRetry(registry, greeterActorID, NewGreeterMessage("Hive User"), 3, 50*time.Millisecond)
	if err != nil {
		fmt.Printf("Failed to dispatch message 2: %v\n", err)
	}

	// You can also dispatch messages to a new, uniquely identified actor of a given type.
	// This is useful when you don't care about the specific instance ID, or want a new one each time.
	err = registry.DispatchTypeMessage(greeterActorType, NewGreeterMessage("Ephemeral User"))
	if err != nil {
		fmt.Printf("Failed to dispatch ephemeral message: %v\n", err)
	}

	// Give some time for messages to be processed.
	// In a real application, you would typically have a more robust way to wait
	// for operations to complete, e.g., using response channels for requests.
	time.Sleep(100 * time.Millisecond)

	fmt.Println("All messages dispatched. Shutting down registry.")
	// The defer registry.GracefulStop() will now be called.
}
