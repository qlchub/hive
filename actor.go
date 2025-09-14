package hive

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jxskiss/base62"
)

// Message is the fundamental unit of communication between actors.
// All messages must implement this interface.
type Message interface {
	// Context returns the context associated with this message.
	Context() context.Context
	// SetContext sets the context for this message.
	SetContext(context.Context)
}

// Actor defines the interface for actors that handle messages directly.
// These actors typically process a single message at a time.
type Actor interface {
	// HandleMessage processes an incoming message.
	// The hive.Context provides access to actor-specific resources like its ID, logger, and dispatcher.
	HandleMessage(ctx *Context, msg Message) error
}

// ActorLooper defines the interface for the main loop of a LoopingActor.
// It's responsible for receiving and processing messages from a channel.
type ActorLooper interface {
	// ActorLoop is the primary execution loop for a LoopingActor.
	// It receives messages from msgCh and should run until the channel is closed
	// or the hive.Context's RegistryCtx is done.
	ActorLoop(msgCh <-chan Message) error
}

// LoopingActor defines the interface for actors that manage their own message loop.
// This is useful for actors that need more control over their execution flow,
// such as those interacting with external systems or managing long-lived state.
type LoopingActor interface {
	// NewActorLoop creates and returns an ActorLooper instance.
	// This method is called by the actor system when the LoopingActor is started.
	NewActorLoop(ctx *Context) ActorLooper
}

// Dispatcher defines the interface for sending messages to actors.
// The Registry implements this interface.
type Dispatcher interface {
	// DispatchMessage sends the given message to the actor identified by actorID.
	DispatchMessage(actorID ID, msg Message) error
	// DispatchTypeMessage sends the given message to new actor of the specified type.
	DispatchTypeMessage(actorType Type, msg Message) error
}

// Type represents the category or kind of an actor (e.g., "user", "device").
type Type string

// EntityID represents the unique identifier of an entity within a given actor Type.
type EntityID string

// ID uniquely identifies an actor instance. It combines an actor Type and an EntityID.
// It also contains an internal precomputed uniqueID string for efficient map lookups.
type ID struct {
	Type     Type
	ID       EntityID
	uniqueID string // Internal representation for performance, format: "Type:EntityID"
}

// NewID creates a new actor ID from a Type and an EntityID.
// It precomputes the unique string representation used for internal operations.
func NewID(typ Type, id EntityID) ID {
	uniqueStr := fmt.Sprintf("%s:%s", typ, id)
	return ID{
		Type:     typ,
		ID:       id,
		uniqueID: uniqueStr,
	}
}

// String returns the precomputed unique string representation of the actor ID (e.g., "Type:EntityID").
// This is primarily used for logging and as a key in maps.
func (a ID) String() string {
	if a.uniqueID == "" {
		return fmt.Sprintf("%s:%s", a.Type, a.ID)
	}

	return a.uniqueID
}

func GenerateUID() string {
	uid := uuid.New()
	encodedUID := base62.Encode(uid[:])

	return string(encodedUID)
}
