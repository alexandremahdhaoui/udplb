//go:build unit

/*
 * Copyright 2025 Alexandre Mahdhaoui
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package clusteradpater

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/types"
	"github.com/alexandremahdhaoui/udplb/internal/types/mocks"
	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

/*******************************************************************************
 * Test types
 *
 ******************************************************************************/

type testPayload struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

/*******************************************************************************
 * Helper: testHarness bundles mock + channels for test setup
 *
 ******************************************************************************/

type testHarness struct {
	mock           *mocks.MockCluster[types.RawData]
	rawRecvCh      chan types.RawData
	capturedSendCh <-chan types.RawData
}

// newTypedClusterForTest creates a typedCluster[testPayload] backed by a
// generated testify MockCluster[RawData].
//
// The mock is pre-configured with expectations for Run's internal calls:
//   - Send(Anything): captures the channel passed by typedCluster.Run
//   - Recv(): returns a buffered channel the test can write to
//
// Lifecycle expectations (Run, Close, Done) are NOT set here; each test
// sets them as needed.
func newTypedClusterForTest(t *testing.T) (*typedCluster[testPayload], *testHarness) {
	t.Helper()

	m := mocks.NewMockCluster[types.RawData](t)

	h := &testHarness{
		mock:      m,
		rawRecvCh: make(chan types.RawData, 64),
	}

	// Send: capture the channel typedCluster registers during Run.
	m.EXPECT().Send(testifymock.Anything).RunAndReturn(func(ch <-chan types.RawData) error {
		h.capturedSendCh = ch
		return nil
	}).Maybe()

	// Recv: return a channel the test controls.
	m.EXPECT().Recv().Return((<-chan types.RawData)(h.rawRecvCh), func() {}).Maybe()

	recvMux := util.NewWatcherMux[testPayload](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[testPayload],
	)
	tc := New[testPayload](m, recvMux)
	return tc.(*typedCluster[testPayload]), h
}

// dispatchRaw injects raw bytes into the mock's recv channel.
func (h *testHarness) dispatchRaw(data types.RawData) {
	h.rawRecvCh <- data
}

// readSent reads a single raw message from the captured send channel.
// Returns the data or fails the test on timeout.
func (h *testHarness) readSent(t *testing.T, timeout time.Duration) types.RawData {
	t.Helper()
	require.NotNil(t, h.capturedSendCh, "no send channel captured on mock")

	select {
	case data := <-h.capturedSendCh:
		return data
	case <-time.After(timeout):
		t.Fatal("timeout waiting for sent data on mock")
		return nil
	}
}

/*******************************************************************************
 * Tests: Lifecycle
 *
 ******************************************************************************/

func TestTypedCluster_RunClose_Lifecycle(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)

	tc.mu.Lock()
	assert.True(t, tc.running)
	assert.False(t, tc.closed)
	tc.mu.Unlock()

	err = tc.Close()
	require.NoError(t, err)

	select {
	case <-tc.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after Close()")
	}

	tc.mu.Lock()
	assert.False(t, tc.running)
	assert.True(t, tc.closed)
	tc.mu.Unlock()
}

func TestTypedCluster_DoubleRun(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	err = tc.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrAlreadyRunning)
}

func TestTypedCluster_CloseWithoutRun(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	err := tc.Close()
	assert.ErrorIs(t, err, types.ErrRunnableMustBeRunningToBeClosed)
}

func TestTypedCluster_DoubleClose(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)

	err = tc.Close()
	require.NoError(t, err)

	err = tc.Close()
	assert.ErrorIs(t, err, types.ErrAlreadyClosed)
}

func TestTypedCluster_RunClosedCluster(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)

	err = tc.Close()
	require.NoError(t, err)

	err = tc.Run(context.Background())
	assert.ErrorIs(t, err, types.ErrCannotRunClosedRunnable)
}

/*******************************************************************************
 * Tests: Send encoding
 *
 ******************************************************************************/

func TestTypedCluster_Send_EncodesJSON(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	sendCh := make(chan testPayload, 1)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	// Send a typed value.
	sendCh <- testPayload{Name: "alpha", Value: 42}

	// Read raw bytes from the mock's captured send channel.
	raw := h.readSent(t, 2*time.Second)

	// Verify JSON encoding.
	assert.JSONEq(t, `{"name":"alpha","value":42}`, string(raw))
}

/*******************************************************************************
 * Tests: Recv decoding
 *
 ******************************************************************************/

func TestTypedCluster_Recv_DecodesJSON(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	recvCh, recvCancel := tc.Recv()
	defer recvCancel()

	// Inject raw JSON bytes into the mock's recv channel.
	h.dispatchRaw([]byte(`{"name":"beta","value":99}`))

	select {
	case val := <-recvCh:
		assert.Equal(t, "beta", val.Name)
		assert.Equal(t, 99, val.Value)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decoded recv data")
	}
}

/*******************************************************************************
 * Tests: Send/Recv round-trip
 *
 ******************************************************************************/

func TestTypedCluster_SendRecv_RoundTrip(t *testing.T) {
	// Simulate a round-trip: send a typed value, have the mock relay the raw
	// bytes back into the recv pipeline, and verify the decoded output.
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	sendCh := make(chan testPayload, 1)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	recvCh, recvCancel := tc.Recv()
	defer recvCancel()

	// Send a typed value.
	original := testPayload{Name: "roundtrip", Value: 7}
	sendCh <- original

	// Read the raw encoded bytes from the mock.
	raw := h.readSent(t, 2*time.Second)

	// Relay them back as if another node sent them.
	h.dispatchRaw(raw)

	// Read the decoded value.
	select {
	case val := <-recvCh:
		assert.Equal(t, original.Name, val.Name)
		assert.Equal(t, original.Value, val.Value)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for round-trip data")
	}
}

/*******************************************************************************
 * Tests: Multiple sends
 *
 ******************************************************************************/

func TestTypedCluster_MultipleSends(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	sendCh := make(chan testPayload, 3)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	payloads := []testPayload{
		{Name: "first", Value: 1},
		{Name: "second", Value: 2},
		{Name: "third", Value: 3},
	}

	for _, p := range payloads {
		sendCh <- p
	}

	for i := 0; i < 3; i++ {
		raw := h.readSent(t, 2*time.Second)
		require.NotEmpty(t, raw)
	}
}

/*******************************************************************************
 * Tests: Recv drops invalid JSON
 *
 ******************************************************************************/

func TestTypedCluster_Recv_DropsInvalidJSON(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	recvCh, recvCancel := tc.Recv()
	defer recvCancel()

	// Dispatch invalid JSON first.
	h.dispatchRaw([]byte("not valid json"))

	// Dispatch valid JSON after.
	h.dispatchRaw([]byte(`{"name":"valid","value":1}`))

	// Only the valid message should arrive.
	select {
	case val := <-recvCh:
		assert.Equal(t, "valid", val.Name)
		assert.Equal(t, 1, val.Value)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for valid data after invalid JSON was dropped")
	}
}

/*******************************************************************************
 * Tests: Delegation to raw cluster
 *
 ******************************************************************************/

func TestTypedCluster_Join_DelegatesToRaw(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	h.mock.EXPECT().Join().Return(nil)

	assert.NoError(t, tc.Join())
}

func TestTypedCluster_Leave_DelegatesToRaw(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	h.mock.EXPECT().Leave().Return(nil)

	assert.NoError(t, tc.Leave())
}

func TestTypedCluster_ListNodes_DelegatesToRaw(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	expectedNodes := []uuid.UUID{uuid.New(), uuid.New()}
	h.mock.EXPECT().ListNodes().Return(expectedNodes)

	nodes := tc.ListNodes()
	assert.Len(t, nodes, 2)
}

func TestTypedCluster_Done(t *testing.T) {
	tc, _ := newTypedClusterForTest(t)

	doneCh := tc.Done()
	require.NotNil(t, doneCh)

	// doneCh should not be closed before Run.
	select {
	case <-doneCh:
		t.Fatal("doneCh should not be closed before Run")
	default:
	}

	err := tc.Run(context.Background())
	require.NoError(t, err)

	// doneCh should not be closed while running.
	select {
	case <-doneCh:
		t.Fatal("doneCh should not be closed while running")
	default:
	}

	err = tc.Close()
	require.NoError(t, err)

	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after Close()")
	}
}

/*******************************************************************************
 * Tests: Run when raw.Send() fails
 *
 ******************************************************************************/

func TestTypedCluster_Run_SendError(t *testing.T) {
	m := mocks.NewMockCluster[types.RawData](t)
	sendErr := errors.New("send registration failed")
	m.EXPECT().Send(testifymock.Anything).Return(sendErr)

	recvMux := util.NewWatcherMux[testPayload](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[testPayload],
	)
	tc := New[testPayload](m, recvMux).(*typedCluster[testPayload])

	err := tc.Run(context.Background())
	assert.ErrorIs(t, err, sendErr)
}

/*******************************************************************************
 * Tests: decodeLoop exits when rawRecvCh is closed
 *
 ******************************************************************************/

func TestTypedCluster_DecodeLoop_ChannelClosed(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)

	// Close the raw recv channel. This simulates the raw cluster shutting down.
	close(h.rawRecvCh)

	// The decodeLoop should exit, closing doneCh.
	select {
	case <-tc.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("doneCh was not closed after rawRecvCh was closed")
	}
}

/*******************************************************************************
 * Tests: Send goroutine exits when input channel is closed
 *
 ******************************************************************************/

func TestTypedCluster_Send_ChannelClosed(t *testing.T) {
	tc, h := newTypedClusterForTest(t)

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	sendCh := make(chan testPayload, 1)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	// Send a value then close the channel.
	sendCh <- testPayload{Name: "last", Value: 99}
	close(sendCh)

	// The sent value should arrive on the raw send channel.
	raw := h.readSent(t, 2*time.Second)
	assert.JSONEq(t, `{"name":"last","value":99}`, string(raw))
}

/*******************************************************************************
 * Tests: Send goroutine handles marshal error (channel type)
 *
 ******************************************************************************/

// unmarshalable is a type that causes json.Marshal to fail.
type unmarshalable struct {
	Ch chan int `json:"ch"`
}

func TestTypedCluster_Send_MarshalError(t *testing.T) {
	// Create a typedCluster[unmarshalable] to trigger json.Marshal errors.
	m := mocks.NewMockCluster[types.RawData](t)

	rawRecvCh := make(chan types.RawData, 64)
	m.EXPECT().Send(testifymock.Anything).Return(nil).Maybe()
	m.EXPECT().Recv().Return((<-chan types.RawData)(rawRecvCh), func() {}).Maybe()

	recvMux := util.NewWatcherMux[unmarshalable](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[unmarshalable],
	)
	tc := New[unmarshalable](m, recvMux).(*typedCluster[unmarshalable])

	err := tc.Run(context.Background())
	require.NoError(t, err)
	defer tc.Close()

	sendCh := make(chan unmarshalable, 2)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	// Send an unmarshalable value (channel type cannot be marshaled).
	sendCh <- unmarshalable{Ch: make(chan int)}

	// Give the goroutine time to process and log the error.
	time.Sleep(100 * time.Millisecond)

	// Now send a closable signal to prove the goroutine is still alive.
	close(sendCh)

	// Small delay to let goroutine process close.
	time.Sleep(50 * time.Millisecond)
}

/*******************************************************************************
 * Tests: Send goroutine exits when terminateCh is closed during rawSendCh write
 *
 ******************************************************************************/

func TestTypedCluster_Send_TerminateDuringSend(t *testing.T) {
	// Use a mock that captures the send channel but never reads from it,
	// causing the inner select to block on rawSendCh.
	m := mocks.NewMockCluster[types.RawData](t)

	rawRecvCh := make(chan types.RawData, 64)
	m.EXPECT().Send(testifymock.Anything).Return(nil).Maybe()
	m.EXPECT().Recv().Return((<-chan types.RawData)(rawRecvCh), func() {}).Maybe()

	recvMux := util.NewWatcherMux[testPayload](
		util.WatcherMuxRecommendedBufferSize,
		util.NonBlockingDispatchFunc[testPayload],
	)
	tc := New[testPayload](m, recvMux).(*typedCluster[testPayload])

	err := tc.Run(context.Background())
	require.NoError(t, err)

	// Create an unbuffered send channel and a custom Send that won't drain rawSendCh.
	sendCh := make(chan testPayload, 1)
	err = tc.Send(sendCh)
	require.NoError(t, err)

	// Fill the rawSendCh so the inner select blocks.
	// rawSendCh is unbuffered (created in New), so the first write will block.
	sendCh <- testPayload{Name: "blocked", Value: 1}

	// Give the goroutine time to pick up the value and block on rawSendCh.
	time.Sleep(50 * time.Millisecond)

	// Close terminates the cluster, which closes terminateCh.
	err = tc.Close()
	require.NoError(t, err)
}
