package hive

import (
	"context"
	"log/slog"
)

// Context provides an actor with access to its environment, including its ID,
// logger, the registry's context, and a dispatcher for sending messages.
// It is passed to actor methods like HandleMessage or NewActorLoop.
type Context struct {
	logger        *slog.Logger       // Logger specific to this actor instance.
	actorID       ID                 // The unique identifier of this hive.
	registryCtx   context.Context    // The context of the actor registry, used for cancellation signals.
	processCtx    context.Context    // The context for this specific actor process, cancelled on actor shutdown.
	processCancel context.CancelFunc // Function to cancel the processCtx.
	dispatcher    Dispatcher         // Interface to dispatch messages to other actors.
}

// NewContext creates a new actor context.
// This function is primarily intended for internal use by the actor registry
// when creating new actor processes, and for setting up contexts in tests.
func NewContext(logger *slog.Logger, actorID ID, registryCtx context.Context, processCtx context.Context, processCancel context.CancelFunc, dispatcher Dispatcher) *Context {
	return &Context{
		logger:        logger,
		actorID:       actorID,
		registryCtx:   registryCtx,
		processCtx:    processCtx,
		processCancel: processCancel,
		dispatcher:    dispatcher,
	}
}

// Logger returns the slog.Logger associated with this actor's context.
// This logger is typically pre-configured with actor-specific fields like actor_type and actor_entity_id.
func (c *Context) Logger() *slog.Logger {
	return c.logger
}

// ID returns the unique identifier (hive.ID) of the hive.
func (c *Context) ID() ID {
	return c.actorID
}

// RegistryCtx returns the context.Context of the actor registry.
// Actors can use this context to listen for shutdown signals from the registry.
// When this context is Done, the actor should clean up and terminate.
func (c *Context) RegistryCtx() context.Context {
	return c.registryCtx
}

// ProcessCtx returns the context.Context for this specific actor process.
// This context is cancelled when the individual actor process begins its shutdown.
// Actors should use this context for operations that need to be cancelled when the actor itself is stopping.
func (c *Context) ProcessCtx() context.Context {
	return c.processCtx
}

// Dispatcher returns the hive.Dispatcher instance associated with this actor's context.
// Actors can use this dispatcher to send messages to other actors within the same registry.
func (c *Context) Dispatcher() Dispatcher {
	return c.dispatcher
}

// Shutdown cancels the context for this specific actor process, signaling it to stop.
func (c *Context) Shutdown() {
	c.processCancel()
}
