package hive_test

import (
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qlchub/hive"
	"github.com/qlchub/hive/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testSimpleActor struct {
	mock.Mock
	handleMessageFunc func(ctx *hive.Context, msg hive.Message) error
}

func (a *testSimpleActor) HandleMessage(ctx *hive.Context, msg hive.Message) error {
	if a.handleMessageFunc != nil {
		return a.handleMessageFunc(ctx, msg)
	}
	args := a.Called(ctx, msg)
	return args.Error(0)
}

func TestRegistry_MustRegisterActor_DuplicateRegistration(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	actorType := hive.Type("duplicateTestActor")
	producer := func() hive.Actor { return mocks.NewMockActor(t) }

	reg.MustRegisterActor(actorType, producer)

	assert.PanicsWithValue(
		t,
		"hive.Registry.MustRegisterActor: actor type already registered [type=duplicateTestActor]",
		func() {
			reg.MustRegisterActor(actorType, producer)
		},
		"Registering the same actor type twice should panic",
	)
}

func TestRegistry_MustRegisterLoopingActor_DuplicateRegistration(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	actorType := hive.Type("duplicateTestLoopingActor")
	producer := func() hive.LoopingActor { return mocks.NewMockLoopingActor(t) }

	reg.MustRegisterLoopingActor(actorType, producer)

	assert.PanicsWithValue(
		t,
		"hive.Registry.MustRegisterLoopingActor: looping actor type already registered [type=duplicateTestLoopingActor]",
		func() {
			reg.MustRegisterLoopingActor(actorType, producer)
		},
		"Registering the same looping actor type twice should panic",
	)
}

func TestRegistry_DispatchMessage_UnregisteredType(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	actorID := hive.NewID("unregisteredType", "id1")
	msg := hive.NewBaseMessage()

	err := reg.DispatchMessage(actorID, msg)

	require.Error(t, err)
	assert.Equal(t, "hive.Registry.DispatchMessage: actor type not registered [actor_type=unregisteredType]", err.Error())
}

func TestRegistry_DispatchMessage_InvalidActorID(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	invalidID := hive.ID{Type: "someType", ID: ""} // Invalid ID
	msg := hive.NewBaseMessage()

	err := reg.DispatchMessage(invalidID, msg)

	require.Error(t, err)
	assert.Equal(t, "hive.Registry.DispatchMessage: actor id is invalid [actor_id=;actor_type=someType]", err.Error())

	invalidID = hive.ID{Type: "", ID: "someID"} // Invalid Type
	err = reg.DispatchMessage(invalidID, msg)

	require.Error(t, err)
	assert.Equal(t, "hive.Registry.DispatchMessage: actor id is invalid [actor_id=someID;actor_type=]", err.Error())
}

func TestRegistry_DispatchMessage_ConcurrentCreation(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	actorType := hive.Type("concurrentCreationTest")
	actorID := hive.NewID(actorType, "entity1")
	msg := hive.NewBaseMessage()

	var producerCallCount int32
	producer := func() hive.Actor {
		atomic.AddInt32(&producerCallCount, 1)
		mockActor := mocks.NewMockActor(t)
		mockActor.On("HandleMessage", mock.Anything, msg).Return(nil)
		return mockActor
	}
	reg.MustRegisterActor(actorType, producer)

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			err := reg.DispatchMessage(actorID, msg)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&producerCallCount), "Producer should be called only once")
	reg.GracefulStop()
}

func TestRegistry_GracefulStop(t *testing.T) {
	reg := hive.NewRegistry(t.Context())
	reg.SetLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	actorType := hive.Type("gracefulStopTest")
	actorID := hive.NewID(actorType, "entity1")
	msg := hive.NewBaseMessage()

	actorStarted := make(chan struct{})
	actorHandlerDone := make(chan struct{})

	producer := func() hive.Actor {
		sa := &testSimpleActor{}
		sa.handleMessageFunc = func(ctx *hive.Context, m hive.Message) error {
			close(actorStarted)
			<-ctx.RegistryCtx().Done()
			close(actorHandlerDone)
			return nil
		}
		return sa
	}
	reg.MustRegisterActor(actorType, producer)

	err := reg.DispatchMessage(actorID, msg)
	require.NoError(t, err)

	<-actorStarted

	stopDone := make(chan struct{})
	go func() {
		reg.GracefulStop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("GracefulStop did not complete in time")
	}

	select {
	case <-actorHandlerDone:
	case <-time.After(1 * time.Second):
		t.Fatal("Actor handler did not unblock after GracefulStop")
	}

	err = reg.DispatchMessage(actorID, msg)
	require.Error(t, err, "dispatching to a new actor after graceful stop should return an error")
	assert.Contains(t, err.Error(), "registry is shutting down", "error message should indicate registry shutdown")
}
