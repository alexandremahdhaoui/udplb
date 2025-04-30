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
package util

import "sync"

/*******************************************************************************
 * LinkedList
 *
 * Properties:
 * - Doubly linked list.
 * - Thread-safe.
 *
 * Expected behavior of Append when capacity is set:
 * - Let `c` the capacity of a LinkedList.
 * - Let `n` the current length of a LinkedList.
 * - If c > 0 && c == n; then calling Append will delete the head and insert
 *   the item at the tail.
 ******************************************************************************/

type LinkedList[T any] struct {
	head     *LLNode[T]
	tail     *LLNode[T]
	length   uint
	capacity uint
	mu       *sync.Mutex
}

func (l *LinkedList[T]) Head() *LLNode[T] {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.head
}

func (l *LinkedList[T]) Tail() *LLNode[T] {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.tail
}

func (l *LinkedList[T]) Length() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(l.length)
}

func (l *LinkedList[T]) Capacity() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(l.capacity)
}

// Append will create a new node containing the data and append it at the end
// of the linked list.
func (l *LinkedList[T]) Append(data T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// capacity is set && length is equal to capcity.
	if l.capacity != 0 && l.length == l.capacity {
		l.popHead()
	}

	// -- critical section
	// This section should not create a deadlock as the l.tail of type *LLNode[T]
	// never blocks while locking.
	oldTail := l.tail
	oldTail.mu.Lock()
	oldTail.next = &LLNode[T]{
		previous: oldTail,
		next:     nil,
		data:     data,
		mu:       &sync.Mutex{},
	}
	oldTail.mu.Unlock()
	// -- end critical section

	l.tail = l.tail.next
	l.length += 1
}

func (l *LinkedList[T]) popHead() {
	switch l.length {
	case 0: // do nothing
		return
	case 1: // after popping the last element, the list is empty.
		l.head = nil
		l.tail = nil
	default: // length > 1
		l.head = l.head.next
	}
	l.length -= 1
}

func NewLinkedList[T any](
	capacity uint,
) *LinkedList[T] {
	return &LinkedList[T]{
		head:     nil,
		tail:     nil,
		length:   0,
		capacity: capacity,
		mu:       &sync.Mutex{},
	}
}

/*******************************************************************************
 * LLNode is thread-safe.
 *
 *
 ******************************************************************************/

type LLNode[T any] struct {
	previous, next *LLNode[T]
	data           T
	mu             *sync.Mutex
}

func (n *LLNode[T]) Previous() *LLNode[T] {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.previous
}

func (n *LLNode[T]) Next() *LLNode[T] {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.next
}

func (n *LLNode[T]) Data() T {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.data
}
