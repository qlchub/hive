package hive_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/qlchub/hive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stringableMessage struct {
	*hive.BaseMessage
	content string
}

func newStringableMessage(ctx context.Context, content string) *stringableMessage {
	return &stringableMessage{BaseMessage: hive.NewBaseMessageWithContext(ctx), content: content}
}

func (m *stringableMessage) String() string {
	return m.content
}

type nonStringableMessage struct {
	*hive.BaseMessage
}

func newNonStringableMessage(ctx context.Context) *nonStringableMessage {
	return &nonStringableMessage{BaseMessage: hive.NewBaseMessageWithContext(ctx)}
}

func TestCreateMessageHandlerWithFilter(t *testing.T) {
	baseCtx := context.Background()
	msgStringable := newStringableMessage(baseCtx, "test_message_content")
	msgNonStringable := newNonStringableMessage(baseCtx)
	handlerErr := errors.New("handler error")

	tests := []struct {
		name              string
		message           hive.Message
		logger            *slog.Logger
		filterFunc        func(msg hive.Message) bool
		actualHandlerFunc func(msg hive.Message) error
		expectHandlerCall bool
		expectedError     error
		expectedLogOutput []string
	}{
		{
			name:    "Nil logger, filter allows, handler succeeds",
			message: msgStringable,
			logger:  nil,
			filterFunc: func(msg hive.Message) bool {
				return true
			},
			actualHandlerFunc: func(msg hive.Message) error {
				return nil
			},
			expectHandlerCall: true,
			expectedError:     nil,
		},
		{
			name:    "Filter allows, handler succeeds",
			message: msgStringable,
			logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
			filterFunc: func(msg hive.Message) bool {
				return true
			},
			actualHandlerFunc: func(msg hive.Message) error {
				return nil
			},
			expectHandlerCall: true,
			expectedError:     nil,
			expectedLogOutput: []string{"message handling completed", msgStringable.String()},
		},
		{
			name:    "Filter blocks, handler not called",
			message: msgStringable,
			logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
			filterFunc: func(msg hive.Message) bool {
				return false
			},
			actualHandlerFunc: func(msg hive.Message) error {
				t.Error("actualHandler should not be called when filter returns false")
				return nil
			},
			expectHandlerCall: false,
			expectedError:     nil,
			expectedLogOutput: []string{"message filtered out", msgStringable.String()},
		},
		{
			name:    "Filter allows, handler returns error",
			message: msgStringable,
			logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
			filterFunc: func(msg hive.Message) bool {
				return true
			},
			actualHandlerFunc: func(msg hive.Message) error {
				return handlerErr
			},
			expectHandlerCall: true,
			expectedError:     handlerErr,
			expectedLogOutput: []string{"message handling failed", msgStringable.String(), handlerErr.Error()},
		},
		{
			name:    "Message without String() method, handler succeeds",
			message: msgNonStringable,
			logger:  slog.New(slog.NewJSONHandler(io.Discard, nil)),
			filterFunc: func(msg hive.Message) bool {
				return true
			},
			actualHandlerFunc: func(msg hive.Message) error {
				return nil
			},
			expectHandlerCall: true,
			expectedError:     nil,
			expectedLogOutput: []string{"message handling completed", "*hive_test.nonStringableMessage"},
		},
		{
			name:       "No filter (nil), handler succeeds",
			message:    msgStringable,
			logger:     slog.New(slog.NewJSONHandler(io.Discard, nil)),
			filterFunc: nil,
			actualHandlerFunc: func(msg hive.Message) error {
				return nil
			},
			expectHandlerCall: true,
			expectedError:     nil,
			expectedLogOutput: []string{"message handling completed", msgStringable.String()},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			testLogger := tc.logger
			if tc.logger == nil {
				testLogger = slog.New(slog.DiscardHandler)
			} else if len(tc.expectedLogOutput) > 0 {
				testLogger = slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
			}

			handlerCallTracker := 0
			trackedActualHandler := func(msg hive.Message) error {
				handlerCallTracker++
				return tc.actualHandlerFunc(msg)
			}

			wrappedHandler := hive.CreateMessageHandlerWithFilter(trackedActualHandler, testLogger, tc.filterFunc)
			err := wrappedHandler(tc.message)

			if tc.expectHandlerCall {
				assert.Equal(t, 1, handlerCallTracker, "actualHandler call count mismatch")
			} else {
				assert.Equal(t, 0, handlerCallTracker, "actualHandler should not have been called")
			}

			if tc.expectedError != nil {
				require.Error(t, err)
				assert.Equal(t, tc.expectedError, err)
			} else {
				assert.NoError(t, err)
			}

			if len(tc.expectedLogOutput) > 0 {
				logOutput := logBuffer.String()
				for _, expectedSubstring := range tc.expectedLogOutput {
					assert.Contains(t, logOutput, expectedSubstring, "Log output mismatch")
				}
			}
		})
	}
}

func TestExtractContext(t *testing.T) {
	customCtxKey := "customKey"
	customCtxValue := "customValue"
	//lint:ignore SA1029 allowed in test
	ctxWithVal := context.WithValue(context.Background(), customCtxKey, customCtxValue)

	tests := []struct {
		name        string
		message     hive.Message
		expectedCtx context.Context
	}{
		{
			name:        "Nil message",
			message:     nil,
			expectedCtx: context.Background(),
		},
		{
			name:        "Message with background context",
			message:     hive.NewBaseMessage(),
			expectedCtx: context.Background(),
		},
		{
			name:        "Message with custom context",
			message:     hive.NewBaseMessageWithContext(ctxWithVal),
			expectedCtx: ctxWithVal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			extractedCtx := hive.ExtractContext(tc.message)
			assert.Equal(t, tc.expectedCtx, extractedCtx)

			if tc.expectedCtx == ctxWithVal {
				assert.Equal(t, customCtxValue, extractedCtx.Value(customCtxKey))
			}
		})
	}
}
