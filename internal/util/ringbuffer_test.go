package util_test

import (
	"testing"
	"time"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestRingBuffer(t *testing.T) {
	t.Run("should create a new ring buffer", func(t *testing.T) {
		rb := util.NewRingBuffer[int](10)
		assert.NotNil(t, rb)
	})

	t.Run("should write and read data", func(t *testing.T) {
		rb := util.NewRingBuffer[int](10)
		rb.Write(1)
		rb.Write(2)
		rb.Write(3)

		item := rb.Next()
		assert.Equal(t, 1, item)

		item = rb.Next()
		assert.Equal(t, 2, item)

		item = rb.Next()
		assert.Equal(t, 3, item)
	})

	t.Run("should block when reading from an empty buffer", func(t *testing.T) {
		rb := util.NewRingBuffer[int](10)

		nextDone := make(chan struct{})
		go func() {
			rb.Next()
			close(nextDone)
		}()

		select {
		case <-nextDone:
			t.Fatal("Next() did not block on empty buffer")
		case <-time.After(100 * time.Millisecond):
			// Test passed, Next() blocked as expected.
		}
	})

	t.Run("should overwrite old data when the buffer is full", func(t *testing.T) {
		rb := util.NewRingBuffer[int](3)
		rb.Write(1)
		rb.Write(2)
		rb.Write(3)
		rb.Write(4)

		item := rb.Next()
		assert.Equal(t, 2, item)

		item = rb.Next()
		assert.Equal(t, 3, item)

		item = rb.Next()
		assert.Equal(t, 4, item)
	})
}
