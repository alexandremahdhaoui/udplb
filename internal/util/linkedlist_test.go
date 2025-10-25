package util_test

import (
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestLinkedList(t *testing.T) {
	t.Run("should create a new linked list", func(t *testing.T) {
		ll := util.NewLinkedList[int](10)
		assert.NotNil(t, ll)
		assert.Equal(t, 0, ll.Length())
		assert.Equal(t, 10, ll.Capacity())
		assert.Nil(t, ll.Head())
		assert.Nil(t, ll.Tail())
	})

	t.Run("should append data to the linked list", func(t *testing.T) {
		ll := util.NewLinkedList[int](10)
		ll.Append(1)
		ll.Append(2)
		ll.Append(3)

		assert.Equal(t, 3, ll.Length())
		assert.Equal(t, 1, ll.Head().Data())
		assert.Equal(t, 3, ll.Tail().Data())
	})

	t.Run("should remove the head when the list is full", func(t *testing.T) {
		ll := util.NewLinkedList[int](2)
		ll.Append(1)
		ll.Append(2)
		ll.Append(3)

		assert.Equal(t, 2, ll.Length())
		assert.Equal(t, 2, ll.Head().Data())
		assert.Equal(t, 3, ll.Tail().Data())
	})

	t.Run("should correctly link nodes", func(t *testing.T) {
		ll := util.NewLinkedList[int](10)
		ll.Append(1)
		ll.Append(2)
		ll.Append(3)

		head := ll.Head()
		assert.Equal(t, 1, head.Data())
		assert.Nil(t, head.Previous())
		assert.NotNil(t, head.Next())

		middle := head.Next()
		assert.Equal(t, 2, middle.Data())
		assert.Equal(t, head, middle.Previous())
		assert.Equal(t, ll.Tail(), middle.Next())

		tail := ll.Tail()
		assert.Equal(t, 3, tail.Data())
		assert.NotNil(t, tail.Previous())
		assert.Nil(t, tail.Next())
	})
}
