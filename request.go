package hive

import "context"

// ActorRequest is a generic request message that embeds BaseMessage.
// It can be used as a base for more specific request types.
type ActorRequest struct {
	*BaseMessage
}

// ActorErrorRequest represents a request message that expects an error as a response.
// It embeds BaseMessage and includes a channel for the error response.
type ActorErrorRequest struct {
	*BaseMessage
	responseChan chan<- error
}

// NewErrorRequest creates a new ActorErrorRequest.
// It initializes the embedded BaseMessage with the provided context and sets the response channel.
func NewErrorRequest(ctx context.Context, ch chan<- error) ActorErrorRequest {
	return ActorErrorRequest{
		BaseMessage:  NewBaseMessageWithContext(ctx),
		responseChan: ch,
	}
}

// Response returns the write-only channel for sending an error response.
// The actor handling this request should send an error (or nil if successful) to this channel.
func (r *ActorErrorRequest) Response() chan<- error {
	return r.responseChan
}

// DataResponse is a generic struct used to send a data payload or an error back
// in response to an ActorDataRequest.
type DataResponse[T any] struct {
	Data  T
	Error error
}

// ActorDataRequest represents a request message that expects a typed data response
// or an error. It embeds BaseMessage and includes a channel for the DataResponse.
// The type T specifies the expected type of the data in the response.
type ActorDataRequest[T any] struct {
	*BaseMessage
	responseChan chan<- DataResponse[T]
}

// NewDataRequest creates a new ActorDataRequest of a specific type T.
// It initializes the embedded BaseMessage with the provided context and sets the response channel.
func NewDataRequest[T any](ctx context.Context, ch chan<- DataResponse[T]) ActorDataRequest[T] {
	return ActorDataRequest[T]{
		BaseMessage:  NewBaseMessageWithContext(ctx),
		responseChan: ch,
	}
}

// Response returns the write-only channel for sending a DataResponse[T].
// The actor handling this request should send a DataResponse (containing either data or an error)
// to this channel.
func (r *ActorDataRequest[T]) Response() chan<- DataResponse[T] {
	return r.responseChan
}
