package hive

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jxskiss/base62"
)

// actorProducerFunc is a function type that produces an actor instance.
// It's used internally by the Registry to create new actors.
// The returned `any` type is expected to be either an `Actor` or a `LoopingActor`.
type actorProducerFunc func() any

// actorUnit holds the producer function and type information for a registered actor type.
// It's used internally by the Registry.
type actorUnit struct {
	producer  actorProducerFunc // Function to create a new instance of the actor.
	isLooping bool              // True if the actor is a LoopingActor, false for a simple Actor.
	opts      Options
}

// Registry manages the lifecycle of actors. It allows registering actor types,
// dispatching messages to actors, and gracefully shutting down all active actors.
// Each actor instance is run in its own goroutine (process).
type Registry struct {
	logger         *slog.Logger
	actorTypes     map[Type]actorUnit
	processes      map[string]*process
	mu             sync.RWMutex
	registryCtx    context.Context
	registryCancel context.CancelFunc
	processWg      sync.WaitGroup
}

// NewRegistry creates and initializes a new actor Registry.
// It sets up a root context that can be used to signal shutdown to all actors
// managed by this registry.
func NewRegistry(ctx context.Context) *Registry {
	ctx, cancel := context.WithCancel(ctx)

	return &Registry{
		logger:         slog.Default(),
		actorTypes:     make(map[Type]actorUnit),
		registryCtx:    ctx,
		registryCancel: cancel,
		processes:      make(map[string]*process),
		processWg:      sync.WaitGroup{},
	}
}

// MustRegisterActor registers a new actor type with the registry.
// An actor type is defined by its Type and a producer function that creates instances of the actor.
// This version is for actors implementing the `Actor` interface (simple, non-looping actors).
// It panics if the actorType is already registered.
//
// Parameters:
//   - actorType: The Type identifier for this kind of actor.
//   - producer: A function that returns a new instance of an object implementing the Actor interface.
func (r *Registry) MustRegisterActor(actorType Type, producer func() Actor, opts ...Option) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actorTypes[actorType]; exists {
		panic(fmt.Sprintf("hive.Registry.MustRegisterActor: actor type already registered [type=%s]", actorType))
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	r.actorTypes[actorType] = actorUnit{
		producer:  func() any { return producer() },
		isLooping: false,
		opts:      options,
	}
	r.logger.Debug("hive.Registry.MustRegisterActor: actor type registered", "type", actorType, "is_looping", false)
}

func (r *Registry) MustRegisterActorAndRun(actorType Type, producer func() Actor, opts ...Option) (ID, error) {
	r.MustRegisterActor(actorType, producer, opts...)

	return r.RunActor(actorType)
}

// portChargingStatusManagerID, err := actorRegistry.RunActor(elomap.PortChargingStatusManagerActorType)
// if err != nil {
// 	return fmt.Errorf("elomap.run: failed to start ports actor: %w", err)
// }

// MustRegisterLoopingActor registers a new looping actor type with the registry.
// A looping actor type is defined by its Type and a producer function that creates instances
// of actors implementing the `LoopingActor` interface.
// It panics if the actorType is already registered.
//
// Parameters:
//   - actorType: The Type identifier for this kind of actor.
//   - producer: A function that returns a new instance of an object implementing the LoopingActor interface.
func (r *Registry) MustRegisterLoopingActor(actorType Type, producer func() LoopingActor, opts ...Option) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.actorTypes[actorType]; exists {
		panic(fmt.Sprintf("hive.Registry.MustRegisterLoopingActor: looping actor type already registered [type=%s]", actorType))
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(&options)
	}

	r.actorTypes[actorType] = actorUnit{
		producer:  func() any { return producer() },
		isLooping: true,
		opts:      options,
	}
	r.logger.Debug("hive.Registry.MustRegisterLoopingActor: looping actor type registered", "type", actorType, "is_looping", true)
}

func (r *Registry) MustRegisterLoopingActorAndRun(actorType Type, producer func() LoopingActor, opts ...Option) (ID, error) {
	r.MustRegisterLoopingActor(actorType, producer, opts...)

	return r.RunActor(actorType)
}

// DispatchMessage sends a message to the actor identified by the given ID.
// If the actor process does not exist, DispatchMessage will attempt to create and start it,
// using the provided message as the initial message for the new actor.
// If the actor's mailbox is full, an error is returned and the message is dropped.
// If the registry is shutting down and a new actor needs to be created, an error is returned.
//
// Parameters:
//   - id: The ID of the target actor.
//   - msg: The message to send.
//
// Returns an error if the message could not be dispatched (e.g., actor type not registered,
// mailbox full, registry shutting down, invalid actor ID).
func (r *Registry) DispatchMessage(id ID, msg Message) error {
	if id.Type == "" || id.ID == "" {
		return fmt.Errorf("hive.Registry.DispatchMessage: actor id is invalid [actor_id=%s;actor_type=%s]", id.ID, id.Type)
	}

	idStr := id.String()

	r.mu.RLock()
	proc, found := r.processes[idStr]
	r.mu.RUnlock()

	if found {
		return proc.sendMessage(msg)
	}

	r.mu.Lock()
	proc, found = r.processes[idStr]
	if found {
		r.mu.Unlock()
		return proc.sendMessage(msg)
	}

	if r.registryCtx.Err() != nil {
		r.mu.Unlock()
		return fmt.Errorf("hive.Registry.DispatchMessage: registry is shutting down, cannot create new actor [actor_id=%s]", id.String())
	}

	au, typeFound := r.actorTypes[id.Type]
	if !typeFound {
		r.mu.Unlock()
		return fmt.Errorf("hive.Registry.DispatchMessage: actor type not registered [actor_type=%s]", id.Type)
	}

	proc = newProcess(r, id, au, r.registryCtx, r)
	r.processes[idStr] = proc
	r.processWg.Add(1)

	go proc.run(msg)

	r.mu.Unlock()
	return nil
}

func (r *Registry) DispatchTypeMessage(typ Type, msg Message) error {
	return r.DispatchMessage(NewID(typ, EntityID(GenerateUID())), msg)
}

// removeProcess is an internal method called by an actor's process (goroutine) when it exits.
// It removes the actor process from the registry's active processes map.
// This method requires the actorID to have a valid uniqueID.
func (r *Registry) removeProcess(actorID ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if actorID.Type == "" || actorID.ID == "" {
		r.logger.Error("hive.Registry.removeProcess: called with invalid actorID", slog.Any("type", actorID.Type), slog.Any("id", actorID.ID))
		return
	}

	delete(r.processes, actorID.String())
	r.logger.Debug("hive.Registry.removeProcess: process removed from registry", slog.String("actor_id", actorID.String()))
}

// GracefulStop signals all active actor processes to shut down and waits for them to complete.
// It cancels the registry's context, which actors should listen to, and then waits on
// a sync.WaitGroup for all actor goroutines to finish.
func (r *Registry) GracefulStop() {
	r.logger.Debug("hive.Registry.GracefulStop: registry stopping")
	r.registryCancel()
	r.processWg.Wait()
	r.logger.Debug("hive.Registry.GracefulStop: registry stopped")
}

// SetLogger sets the logger for the Registry.
// This allows users to provide a custom slog.Logger instance for all registry-related logging.
func (r *Registry) SetLogger(logger *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.logger = logger
}

// RunActor creates and starts a new actor instance of the specified type.
// This method is used to explicitly start an actor without sending an initial message.
// The actor will be assigned a new unique entity ID.
//
// Parameters:
//   - actorType: The Type identifier for the kind of actor to run.
//
// Returns:
//   - The ID of the newly created and started actor.
//   - An error if the actor type is not registered or if the registry is shutting down.
func (r *Registry) RunActor(actorType Type) (ID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.registryCtx.Err() != nil {
		return ID{}, fmt.Errorf("hive.Registry.RunActor: registry is shutting down, cannot create new actor [actor_type=%s]", actorType)
	}

	actorUnit, typeFound := r.actorTypes[actorType]
	if !typeFound {
		return ID{}, fmt.Errorf("hive.Registry.RunActor: actor type not registered [actor_type=%s]", actorType)
	}

	newActorID := NewID(actorType, EntityID(r.generateUniqueUID()))
	proc := newProcess(r, newActorID, actorUnit, r.registryCtx, r)
	r.processes[newActorID.String()] = proc
	r.processWg.Add(1)

	go proc.run(nil)

	return newActorID, nil
}

// generateUniqueUID generates a unique uuid string
func (r *Registry) generateUniqueUID() string {
	uid := uuid.New()
	encodedUID := base62.Encode(uid[:])

	return string(encodedUID)
}
