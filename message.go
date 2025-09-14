package hive

import "context"

// BaseMessage provides a basic implementation of the Message interface.
// It carries a context.Context, which can be used for request-scoped values,
// cancellation signals, and deadlines.
type BaseMessage struct {
	ctx context.Context
}

// NewBaseMessage creates a new BaseMessage with background context.
func NewBaseMessage() *BaseMessage {
	return &BaseMessage{ctx: context.Background()}
}

// NewBaseMessageWithContext creates a new BaseMessage with the given context.
// If ctx is nil, the BaseMessage will store a nil context.
// It is generally recommended to provide a non-nil context.
func NewBaseMessageWithContext(ctx context.Context) *BaseMessage {
	return &BaseMessage{ctx: ctx}
}

// Context returns the context.Context associated with the BaseMessage.
// This fulfills the hive.Message interface. It returns the context that was
// provided during the creation of the BaseMessage, which can be nil.
func (b *BaseMessage) Context() context.Context {
	return b.ctx
}

// SetContext sets the context.Context associated with the BaseMessage.
func (b *BaseMessage) SetContext(ctx context.Context) {
	if b == nil {
		b = &BaseMessage{}
	}
	b.ctx = ctx
}
