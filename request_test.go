package hive_test

import (
	"context"
	"errors"
	"testing"

	"github.com/qlchub/hive"
	"github.com/stretchr/testify/assert"
)

func TestNewErrorRequest(t *testing.T) {
	//lint:ignore SA1029 allowed in test
	expectedCtx := context.WithValue(context.Background(), "key", "error_req_value")
	errChan := make(chan error)

	req := hive.NewErrorRequest(expectedCtx, errChan)

	assert.Equal(t, expectedCtx, req.Context(), "Context should be set in BaseMessage")
	assert.Equal(t, (chan<- error)(errChan), req.Response(), "Response channel should be set")
}

func TestActorErrorRequest_Response(t *testing.T) {
	errChan := make(chan error, 1)
	req := hive.NewErrorRequest(context.Background(), errChan)

	assert.Equal(t, (chan<- error)(errChan), req.Response(), "Response() should return the initially provided channel")

	testErr := errors.New("test error")
	go func() {
		req.Response() <- testErr
	}()

	receivedErr := <-errChan
	assert.Equal(t, testErr, receivedErr)
}

func TestNewDataRequest(t *testing.T) {
	type testDataType struct {
		Value string
	}
	//lint:ignore SA1029 allowed in test
	expectedCtx := context.WithValue(context.Background(), "key", "data_req_value")
	dataChan := make(chan hive.DataResponse[testDataType])

	req := hive.NewDataRequest[testDataType](expectedCtx, dataChan)

	assert.Equal(t, expectedCtx, req.Context(), "Context should be set in BaseMessage")
	assert.Equal(t, (chan<- hive.DataResponse[testDataType])(dataChan), req.Response(), "Response channel should be set")
}

func TestActorDataRequest_Response(t *testing.T) {
	type testDataType struct {
		ID   int
		Name string
	}
	dataChan := make(chan hive.DataResponse[testDataType], 1)
	req := hive.NewDataRequest[testDataType](context.Background(), dataChan)

	assert.Equal(t, (chan<- hive.DataResponse[testDataType])(dataChan), req.Response(), "Response() should return the initially provided channel")

	expectedData := testDataType{ID: 1, Name: "Test Data"}
	expectedErr := errors.New("data error")

	tests := []struct {
		name           string
		responseToSend hive.DataResponse[testDataType]
	}{
		{
			name:           "Successful data response",
			responseToSend: hive.DataResponse[testDataType]{Data: expectedData, Error: nil},
		},
		{
			name:           "Error data response",
			responseToSend: hive.DataResponse[testDataType]{Data: testDataType{}, Error: expectedErr},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			go func() {
				req.Response() <- tc.responseToSend
			}()

			receivedResponse := <-dataChan
			assert.Equal(t, tc.responseToSend, receivedResponse)
		})
	}
}
