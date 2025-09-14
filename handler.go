package hive

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// CreateMessageHandlerWithFilter wraps an actual message handler function with logging,
// timing, and an optional message filter.
//
// Parameters:
//   - actualHandler: The core function that processes the message. It receives the message
//     and returns an error if processing fails.
//   - logger: A slog.Logger instance for logging. If nil, slog.Default() is used.
//   - filter: An optional function that can inspect a message and decide if it should be
//     processed by actualHandler. If filter returns false, the message is skipped.
//     If filter is nil, all messages are passed to actualHandler.
//
// Returns a new function that, when called with a message:
//  1. Logs the start of message handling.
//  2. Applies the filter (if provided). If filtered out, logs and returns nil.
//  3. Calls actualHandler, timing its execution.
//  4. Logs the completion (or failure) of handling, including duration and any error.
//  5. Returns the error from actualHandler.
func CreateMessageHandlerWithFilter(
	actualHandler func(msg Message) error,
	logger *slog.Logger,
	filter func(msg Message) bool,
) func(msg Message) error {
	return func(msg Message) error {
		if logger == nil {
			logger = slog.Default()
		}

		var msgStr string
		if s, ok := msg.(interface{ String() string }); ok {
			msgStr = s.String()
		} else {
			msgStr = fmt.Sprintf("%T", msg)
		}

		logger.Debug("hive.CreateMessageHandlerWithFilter: message handling started", slog.String("message_type", msgStr))

		if filter != nil {
			if !filter(msg) {
				logger.Debug("hive.CreateMessageHandlerWithFilter: message filtered out", slog.String("message_type", msgStr))
				return nil
			}
		}

		startTime := time.Now()
		err := actualHandler(msg)
		duration := time.Since(startTime)

		logArgs := []any{
			slog.Duration("duration", duration),
			slog.String("message_type", msgStr),
		}

		if err != nil {
			logArgs = append(logArgs, slog.String("error", err.Error()))
			logger.Error("hive.CreateMessageHandlerWithFilter: message handling failed", logArgs...)
		} else {
			logger.Debug("hive.CreateMessageHandlerWithFilter: message handling completed", logArgs...)
		}

		return err
	}
}

// ExtractContext safely extracts the context.Context from a Message.
// If the message is nil or its Context() method returns nil,
// context.Background() is returned as a fallback.
func ExtractContext(msg Message) context.Context {
	if msg == nil {
		return context.Background()
	}
	ctx := msg.Context()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
