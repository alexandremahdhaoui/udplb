package util_test

import (
	"errors"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestAnyPtrIsNil(t *testing.T) {
	t.Run("should return true if any pointer is nil", func(t *testing.T) {
		var a *int
		b := 10
		assert.True(t, util.AnyPtrIsNil(a, &b))
	})

	t.Run("should return false if no pointers are nil", func(t *testing.T) {
		a := 10
		b := 20
		assert.False(t, util.AnyPtrIsNil(&a, &b))
	})
}

func TestIgnoreErr(t *testing.T) {
	t.Run("should ignore the error", func(t *testing.T) {
		f := func() (int, error) {
			return 10, errors.New("some error")
		}

		result := util.IgnoreErr(f())
		assert.Equal(t, 10, result)
	})
}
