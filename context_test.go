package hive_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/qlchub/hive"
	"github.com/qlchub/hive/mocks"
	"github.com/stretchr/testify/assert"
)

func TestContext_Getters(t *testing.T) {
	expectedLogger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	expectedActorID := hive.NewID("testType", "testID")
	expectedRegistryCtx := t.Context()
	processCtx, processCancel := context.WithCancel(expectedRegistryCtx)
	defer processCancel()

	mockDispatcher := mocks.NewMockDispatcher(t)

	actorCtx := hive.NewContext(
		expectedLogger,
		expectedActorID,
		expectedRegistryCtx,
		processCtx,
		processCancel,
		mockDispatcher,
	)

	assert.Same(t, expectedLogger, actorCtx.Logger(), "Logger should be the one set during construction")
	assert.Equal(t, expectedActorID, actorCtx.ID(), "ActorID should be the one set during construction")
	assert.Equal(t, expectedRegistryCtx, actorCtx.RegistryCtx(), "RegistryCtx should be the one set during construction")
	assert.Equal(t, processCtx, actorCtx.ProcessCtx(), "ProcessCtx should be the one set during construction")
	assert.Same(t, mockDispatcher, actorCtx.Dispatcher(), "Dispatcher should be the one set during construction")

	mockDispatcher.AssertExpectations(t)
}
