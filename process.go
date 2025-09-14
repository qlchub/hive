package hive

import (
	"context"
	"fmt"
	"log/slog"
)

// process represents the runtime instance of an actor. It manages the actor's
// lifecycle, message channel (mailbox), and the execution of its logic.
// This type is not exported and is managed internally by the Registry.
type process struct {
	registry      *Registry
	actorID       ID
	actorLogic    any
	isLooping     bool
	producer      actorProducerFunc
	msgCh         chan Message
	actorCtx      *Context
	processCancel context.CancelFunc
}

// newProcess creates and initializes a new actor process.
// It is called by the Registry when an actor needs to be started.
// The actor's specific logic (simple or looping) is instantiated via actorUnit.producer.
// The provided registryCtx and dispatcher are used to create the actor's specific context.
func newProcess(reg *Registry, id ID, actorUnit actorUnit, registryCtx context.Context, dispatcher Dispatcher) *process {
	processCtx, processCancel := context.WithCancel(registryCtx)

	actorLogger := reg.logger.With(
		slog.String("actor_type", string(id.Type)),
		slog.String("actor_id", string(id.ID)),
	)

	actorCtx := NewContext(actorLogger, id, registryCtx, processCtx, processCancel, dispatcher)

	p := &process{
		registry:      reg,
		actorID:       id,
		actorLogic:    actorUnit.producer(),
		isLooping:     actorUnit.isLooping,
		producer:      actorUnit.producer,
		msgCh:         make(chan Message, actorUnit.opts.MailboxSize),
		actorCtx:      actorCtx,
		processCancel: processCancel,
	}
	return p
}

// run is the main goroutine for an actor process.
// It starts the actor's message processing loop (for simple actors) or its custom loop (for looping actors).
// It handles the initial message delivery and ensures cleanup (decrementing wait group, removing from registry) upon termination.
// This method is intended to be run in a separate goroutine.
//
// Parameters:
//   - initialMessage: The first message to be delivered to the actor. This is typically the message
//     that triggered the actor's creation. It can be nil if the actor is started without an initial message.
func (p *process) run(initialMessage Message) {
	defer p.registry.processWg.Done()
	defer p.registry.removeProcess(p.actorID)
	defer p.processCancel()
	defer func() {
		close(p.msgCh)
		p.actorCtx.Logger().Debug("hive.process.run: process loop finished")
	}()

	p.actorCtx.Logger().Debug("hive.process.run: process loop started")

	if initialMessage != nil {
		select {
		case p.msgCh <- initialMessage:
		case <-p.actorCtx.ProcessCtx().Done():
			p.actorCtx.Logger().Error("hive.process.run: failed to send initial message to actor, process shutting down")
			return
		default:
			p.actorCtx.Logger().Error("hive.process.run: failed to send initial message to actor, mailbox full or closed")
			return
		}
	}

	if p.isLooping {
		looper := p.actorLogic.(LoopingActor).NewActorLoop(p.actorCtx)
		err := looper.ActorLoop(p.msgCh)
		if err != nil {
			p.actorCtx.Logger().Error("hive.process.run: actor loop exited with error", slog.Any("error", err))
		}
	} else {
		simpleActor := p.actorLogic.(Actor)
		for {
			select {
			case msg, ok := <-p.msgCh:
				if !ok {
					p.actorCtx.Logger().Debug("hive.process.run: actor message channel closed, stopping")
					return
				}

				func() {
					defer func() {
						if r := recover(); r != nil {
							p.actorCtx.Logger().Error("hive.process.run: actor panicked, restarting", "panic", r)
							newActor := p.producer()
							p.actorLogic = newActor
							simpleActor = newActor.(Actor)
						}
					}()

					if err := simpleActor.HandleMessage(p.actorCtx, msg); err != nil {
						p.actorCtx.Logger().Error("hive.process.run: actor message handler error", slog.Any("error", err))
					}
				}()
			case <-p.actorCtx.ProcessCtx().Done():
				p.actorCtx.Logger().Debug("hive.process.run: actor stopping due to process context done")
				return
			}
		}
	}
}

// sendMessage attempts to send a message to the actor's mailbox (msgCh).
// This is a non-blocking send. If the mailbox is full, it returns an error immediately.
//
// Returns:
//   - nil: If the message was successfully sent to the actor's mailbox.
//   - error: If the actor's process is shutting down or if the mailbox is full,
//     in which case the message is not delivered.
func (p *process) sendMessage(msg Message) error {
	actorIDStr := p.actorID.String()

	select {
	case p.msgCh <- msg:
		return nil
	case <-p.actorCtx.ProcessCtx().Done():
		return fmt.Errorf("hive.process.sendMessage: actor failed to send message: process shutting down [actor_id=%s]", actorIDStr)
	default:
		return fmt.Errorf("hive.process.sendMessage: actor mailbox is full [actor_id=%s]", actorIDStr)
	}
}
