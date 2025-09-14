package hive_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qlchub/hive"
	"github.com/qlchub/hive/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestProcess_SimpleActorLifecycle(t *testing.T) {
	actorType := hive.Type("testSimpleType")
	entityID := hive.EntityID("entity1")
	actorID := hive.NewID(actorType, entityID)
	ctx := context.Background()
	msg1 := newMockMessage(ctx, 1, "hello")
	msg2 := newMockMessage(ctx, 2, "world")
	initialMsg := newMockMessage(ctx, 0, "initial")
	errMsg := newMockMessage(ctx, 1, "error message")
	okMsgAfterErr := newMockMessage(ctx, 2, "ok message")

	tests := []struct {
		name               string
		messagesToDispatch []hive.Message
		setupMockActor     func(t *testing.T, mockActor *mocks.MockActor, messages []hive.Message)
		waitTimeout        time.Duration
	}{
		{
			name:               "ProcessMultipleMessagesAndShutdown",
			messagesToDispatch: []hive.Message{msg1, msg2},
			setupMockActor: func(t *testing.T, mockActor *mocks.MockActor, messages []hive.Message) {
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[0]).Return(nil).Once()
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[1]).Return(nil).Once()
			},
			waitTimeout: 100 * time.Millisecond,
		},
		{
			name:               "InitialMessageDeliveryAndSubsequentMessage",
			messagesToDispatch: []hive.Message{initialMsg, msg1},
			setupMockActor: func(t *testing.T, mockActor *mocks.MockActor, messages []hive.Message) {
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[0]).Return(nil).Once()
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[1]).Return(nil).Once()
			},
			waitTimeout: 100 * time.Millisecond,
		},
		{
			name:               "HandlerErrorDoesNotCrashProcessContinues",
			messagesToDispatch: []hive.Message{errMsg, okMsgAfterErr},
			setupMockActor: func(t *testing.T, mockActor *mocks.MockActor, messages []hive.Message) {
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[0]).Return(errors.New("handler failed")).Once()
				mockActor.On("HandleMessage", mock.AnythingOfType("*hive.Context"), messages[1]).Return(nil).Once()
			},
			waitTimeout: 100 * time.Millisecond,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := hive.NewRegistry(t.Context())
			reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
			defer reg.GracefulStop()

			mockActorInstance := mocks.NewMockActor(t)
			tc.setupMockActor(t, mockActorInstance, tc.messagesToDispatch)

			reg.MustRegisterActor(actorType, func() hive.Actor { return mockActorInstance })

			for _, msg := range tc.messagesToDispatch {
				err := reg.DispatchMessage(actorID, msg)
				require.NoError(t, err)
			}

			time.Sleep(tc.waitTimeout)
			mockActorInstance.AssertExpectations(t)
		})
	}
}

func TestProcess_LoopingActorLifecycle(t *testing.T) {
	actorType := hive.Type("testLoopingType")
	entityID := hive.EntityID("entity1")
	actorID := hive.NewID(actorType, entityID)
	ctx := context.Background()
	msg1 := newMockMessage(ctx, 1, "loop hello")
	msg2 := newMockMessage(ctx, 2, "loop world")

	tests := []struct {
		name               string
		messagesToDispatch []hive.Message
		setupLoopingActor  func(t *testing.T, mla *mocks.MockLoopingActor, ml *mocks.MockActorLooper, messages []hive.Message) (*atomic.Int32, chan struct{})
		expectedLoopError  error
		waitTimeout        time.Duration
		triggerStop        func(stopSignal chan struct{})
	}{
		{
			name:               "ProcessMessagesAndShutdown",
			messagesToDispatch: []hive.Message{msg1, msg2},
			setupLoopingActor: func(t *testing.T, mla *mocks.MockLoopingActor, ml *mocks.MockActorLooper, messages []hive.Message) (*atomic.Int32, chan struct{}) {
				var processedCount atomic.Int32
				var capturedActorCtx *hive.Context
				loopStarted := make(chan struct{})

				mla.On("NewActorLoop", mock.AnythingOfType("*hive.Context")).
					Run(func(args mock.Arguments) { capturedActorCtx = args.Get(0).(*hive.Context) }).
					Return(ml).Once()

				ml.On("ActorLoop", mock.AnythingOfType("<-chan hive.Message")).
					Run(func(args mock.Arguments) {
						msgCh := args.Get(0).(<-chan hive.Message)
						close(loopStarted)
						for {
							select {
							case _, ok := <-msgCh:
								if !ok {
									return
								}
								processedCount.Add(1)
							case <-capturedActorCtx.RegistryCtx().Done():
								return
							}
						}
					}).Return(nil).Once()
				return &processedCount, loopStarted
			},
			expectedLoopError: nil,
			waitTimeout:       200 * time.Millisecond,
		},
		{
			name:               "ActorLoopErrorHandledGracefully",
			messagesToDispatch: []hive.Message{msg1},
			setupLoopingActor: func(t *testing.T, mla *mocks.MockLoopingActor, ml *mocks.MockActorLooper, messages []hive.Message) (*atomic.Int32, chan struct{}) {
				var processedCount atomic.Int32
				var capturedActorCtx *hive.Context
				loopStarted := make(chan struct{})
				loopShouldStopManually := make(chan struct{})

				mla.On("NewActorLoop", mock.AnythingOfType("*hive.Context")).
					Run(func(args mock.Arguments) { capturedActorCtx = args.Get(0).(*hive.Context) }).
					Return(ml).Once()

				ml.On("ActorLoop", mock.AnythingOfType("<-chan hive.Message")).
					Run(func(args mock.Arguments) {
						msgCh := args.Get(0).(<-chan hive.Message)
						close(loopStarted)
						select {
						case _, ok := <-msgCh:
							if !ok {
								return
							}
							processedCount.Add(1)
						case <-capturedActorCtx.RegistryCtx().Done():
							return
						case <-loopShouldStopManually:
							return
						}
					}).Return(errors.New("loop exited with error")).Once()
				return &processedCount, loopShouldStopManually
			},
			expectedLoopError: errors.New("loop exited with error"),
			waitTimeout:       200 * time.Millisecond,
			triggerStop: func(stopSignal chan struct{}) {
				close(stopSignal)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := hive.NewRegistry(t.Context())
			reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
			defer reg.GracefulStop()

			mockLoopingActorInst := mocks.NewMockLoopingActor(t)
			mockLooperInst := mocks.NewMockActorLooper(t)

			processedCounter, controlSignal := tc.setupLoopingActor(t, mockLoopingActorInst, mockLooperInst, tc.messagesToDispatch)

			reg.MustRegisterLoopingActor(actorType, func() hive.LoopingActor { return mockLoopingActorInst })

			for _, msg := range tc.messagesToDispatch {
				err := reg.DispatchMessage(actorID, msg)
				require.NoError(t, err)
			}

			if tc.triggerStop != nil {
				time.Sleep(50 * time.Millisecond)
				tc.triggerStop(controlSignal)
			}

			time.Sleep(tc.waitTimeout)

			mockLoopingActorInst.AssertExpectations(t)
			mockLooperInst.AssertExpectations(t)
			if processedCounter != nil && len(tc.messagesToDispatch) > 0 && tc.expectedLoopError == nil {
				assert.Equal(t, int32(len(tc.messagesToDispatch)), processedCounter.Load())
			}
		})
	}
}

func TestProcess_ActorRestartability(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	defer reg.GracefulStop()

	actorTypeLoop := hive.Type("testRestartLoopType")
	entityID := hive.EntityID("entity1")
	actorIDLoop := hive.NewID(actorTypeLoop, entityID)

	loopStopError := errors.New("stopping loop")
	var producerCallCount int32
	secondActorProcessed := make(chan struct{}) // New channel for synchronization

	loopingActorProducer := func() hive.LoopingActor {
		atomic.AddInt32(&producerCallCount, 1)
		currentCallCount := atomic.LoadInt32(&producerCallCount)

		mla := mocks.NewMockLoopingActor(t)
		looper := mocks.NewMockActorLooper(t)
		var capturedActorCtx *hive.Context

		mla.On("NewActorLoop", mock.AnythingOfType("*hive.Context")).
			Run(func(args mock.Arguments) { capturedActorCtx = args.Get(0).(*hive.Context) }).
			Return(looper).Once()

		if currentCallCount == 1 {
			// First instance: runs, receives one message, then returns an error to stop.
			looper.On("ActorLoop", mock.AnythingOfType("<-chan hive.Message")).
				Run(func(args mock.Arguments) {
					msgCh := args.Get(0).(<-chan hive.Message)
					select {
					case <-msgCh:
					case <-capturedActorCtx.RegistryCtx().Done():
					}
				}).Return(loopStopError).Once()
		} else {
			// Second instance: runs, receives one message, signals completion, then exits cleanly.
			looper.On("ActorLoop", mock.AnythingOfType("<-chan hive.Message")).
				Run(func(args mock.Arguments) {
					msgCh := args.Get(0).(<-chan hive.Message)
					select {
					case <-msgCh:
						close(secondActorProcessed) // Signal that the message was processed
					case <-capturedActorCtx.RegistryCtx().Done():
					}
				}).Return(nil).Once()
		}
		return mla
	}

	ctx := context.Background()
	reg.MustRegisterLoopingActor(actorTypeLoop, loopingActorProducer)
	err := reg.DispatchMessage(actorIDLoop, newMockMessage(ctx, 1, "to first looper"))
	require.NoError(t, err)

	// Wait for the first actor process to exit.
	time.Sleep(100 * time.Millisecond)

	// Retry dispatching to trigger the restart.
	var secondDispatchErr error
	for i := 0; i < 10; i++ {
		secondDispatchErr = reg.DispatchMessage(actorIDLoop, newMockMessage(ctx, 2, "to second looper"))
		if secondDispatchErr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, secondDispatchErr, "Failed to dispatch message to the new actor instance after retries")

	// Wait for the second actor to confirm it processed the message, with a timeout.
	select {
	case <-secondActorProcessed:
	// Test passed, the second actor was created and processed its message.
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the second actor to process its message")
	}

	// Final assertion to confirm the producer was called twice.
	assert.Equal(t, int32(2), atomic.LoadInt32(&producerCallCount), "Producer should be called twice for restart")
}

type mockMessage struct {
	*hive.BaseMessage
	id      int
	content string
}

func newMockMessage(ctx context.Context, id int, content string) *mockMessage {
	return &mockMessage{BaseMessage: hive.NewBaseMessageWithContext(ctx), id: id, content: content}
}

func (m *mockMessage) String() string {
	return fmt.Sprintf("mockMessage-%d: %s", m.id, m.content)
}

func TestProcess_Run_InitialMessage_SendFailure_RegistryCtxDone(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	reg.GracefulStop()

	actorType := hive.Type("testCtxDoneBeforeRun")
	actorID := hive.NewID(actorType, "entity1")
	initialMsg := newMockMessage(context.Background(), 0, "initial")

	mockActorInstance := mocks.NewMockActor(t)
	producerCallCount := 0
	reg.MustRegisterActor(actorType, func() hive.Actor {
		producerCallCount++
		return mockActorInstance
	})

	err := reg.DispatchMessage(actorID, initialMsg)
	require.Error(t, err, "DispatchMessage should fail when registry is stopped and new actor creation is attempted")
	assert.Contains(t, err.Error(), "registry is shutting down")
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 0, producerCallCount, "Producer should not have been called")
	mockActorInstance.AssertNotCalled(t, "HandleMessage", mock.Anything, mock.Anything)
}

func TestProcess_SendMessage_RegistryContextDone(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))

	actorType := hive.Type("registryDoneTest")
	actorID := hive.NewID(actorType, "entity1")

	mockActor := mocks.NewMockActor(t)
	actorProcessedMsg := make(chan struct{})

	mockActor.On("HandleMessage", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		close(actorProcessedMsg)
	}).Return(nil).Once()

	reg.MustRegisterActor(actorType, func() hive.Actor { return mockActor })

	err := reg.DispatchMessage(actorID, newMockMessage(context.Background(), 1, "first"))
	require.NoError(t, err)
	<-actorProcessedMsg
	reg.GracefulStop()

	err = reg.DispatchMessage(actorID, newMockMessage(context.Background(), 2, "second_after_stop"))

	require.Error(t, err, "Expected error when sending message after registry context is done")
	assert.ErrorContains(t, err, "registry is shutting down", "Error message should indicate registry shutdown")
	mockActor.AssertNumberOfCalls(t, "HandleMessage", 1)
}
