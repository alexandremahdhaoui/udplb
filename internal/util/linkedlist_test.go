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
package util_test

import (
	"sync"
	"testing"

	"github.com/alexandremahdhaoui/udplb/internal/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkedList_AppendEmptyList(t *testing.T) {
	ll := util.NewLinkedList[int](0)
	require.Equal(t, 0, ll.Length())

	// Append to an empty list must not panic.
	require.NotPanics(t, func() {
		ll.Append(42)
	})

	require.Equal(t, 1, ll.Length())
	require.NotNil(t, ll.Head())
	require.NotNil(t, ll.Tail())
	assert.Equal(t, 42, ll.Head().Data())
	assert.Equal(t, 42, ll.Tail().Data())
}

func TestLinkedList_AppendMultipleElements(t *testing.T) {
	ll := util.NewLinkedList[string](0)

	ll.Append("a")
	ll.Append("b")
	ll.Append("c")

	require.Equal(t, 3, ll.Length())
	assert.Equal(t, "a", ll.Head().Data())
	assert.Equal(t, "c", ll.Tail().Data())

	// Walk from head to tail.
	node := ll.Head()
	assert.Equal(t, "a", node.Data())
	node = node.Next()
	require.NotNil(t, node)
	assert.Equal(t, "b", node.Data())
	node = node.Next()
	require.NotNil(t, node)
	assert.Equal(t, "c", node.Data())
	assert.Nil(t, node.Next())

	// Walk from tail to head.
	node = ll.Tail()
	assert.Equal(t, "c", node.Data())
	node = node.Previous()
	require.NotNil(t, node)
	assert.Equal(t, "b", node.Data())
	node = node.Previous()
	require.NotNil(t, node)
	assert.Equal(t, "a", node.Data())
	assert.Nil(t, node.Previous())
}

func TestLinkedList_CapacityEviction(t *testing.T) {
	ll := util.NewLinkedList[int](3)
	assert.Equal(t, 3, ll.Capacity())

	ll.Append(1)
	ll.Append(2)
	ll.Append(3)
	require.Equal(t, 3, ll.Length())
	assert.Equal(t, 1, ll.Head().Data())
	assert.Equal(t, 3, ll.Tail().Data())

	// Appending a 4th element evicts the head (1).
	ll.Append(4)
	require.Equal(t, 3, ll.Length())
	assert.Equal(t, 2, ll.Head().Data())
	assert.Equal(t, 4, ll.Tail().Data())

	// Appending a 5th element evicts the new head (2).
	ll.Append(5)
	require.Equal(t, 3, ll.Length())
	assert.Equal(t, 3, ll.Head().Data())
	assert.Equal(t, 5, ll.Tail().Data())
}

func TestLinkedList_CapacityOne(t *testing.T) {
	ll := util.NewLinkedList[int](1)

	ll.Append(10)
	require.Equal(t, 1, ll.Length())
	assert.Equal(t, 10, ll.Head().Data())
	assert.Equal(t, 10, ll.Tail().Data())

	// Appending with capacity 1 evicts the only element and inserts the new one.
	ll.Append(20)
	require.Equal(t, 1, ll.Length())
	assert.Equal(t, 20, ll.Head().Data())
	assert.Equal(t, 20, ll.Tail().Data())
}

func TestLinkedList_HeadTailLength(t *testing.T) {
	ll := util.NewLinkedList[int](0)

	// Empty list.
	assert.Equal(t, 0, ll.Length())
	assert.Nil(t, ll.Head())
	assert.Nil(t, ll.Tail())

	ll.Append(1)
	assert.Equal(t, 1, ll.Length())
	require.NotNil(t, ll.Head())
	require.NotNil(t, ll.Tail())
	// Single element: head and tail are the same.
	assert.Equal(t, ll.Head().Data(), ll.Tail().Data())

	ll.Append(2)
	assert.Equal(t, 2, ll.Length())
	assert.Equal(t, 1, ll.Head().Data())
	assert.Equal(t, 2, ll.Tail().Data())
}

func TestLinkedList_ConcurrentAccess(t *testing.T) {
	ll := util.NewLinkedList[int](0)
	const goroutines = 50
	const appendsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < appendsPerGoroutine; j++ {
				ll.Append(base*appendsPerGoroutine + j)
			}
		}(i)
	}

	wg.Wait()

	assert.Equal(t, goroutines*appendsPerGoroutine, ll.Length())
	require.NotNil(t, ll.Head())
	require.NotNil(t, ll.Tail())
}
