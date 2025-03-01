package queue

import (
	"sync"
)

// Node represents a node in the queue
type Node struct {
	value interface{}
	next  *Node
}

// TwoLockQueue is a concurrent queue using two locks as described by Michael and Scott
type TwoLockQueue struct {
	head     *Node
	tail     *Node
	headLock sync.Mutex
	tailLock sync.Mutex
}

// NewTwoLockQueue creates a new empty queue with two locks
func NewTwoLockQueue() *TwoLockQueue {
	// Create a dummy node as described in the paper
	node := &Node{value: nil, next: nil}
	return &TwoLockQueue{
		head: node,
		tail: node,
	}
}

// Enqueue adds a value to the queue
func (q *TwoLockQueue) Enqueue(value interface{}) {
	// Create a new node
	node := &Node{value: value, next: nil}

	// Acquire the tail lock to access Tail
	q.tailLock.Lock()
	defer q.tailLock.Unlock()

	// Link node at the end of the linked list
	q.tail.next = node

	// Swing Tail to node
	q.tail = node
}

// Dequeue removes and returns a value from the queue
// Returns (nil, false) if the queue is empty
func (q *TwoLockQueue) Dequeue() (interface{}, bool) {
	// Acquire the head lock to access Head
	q.headLock.Lock()
	defer q.headLock.Unlock()

	// Read Head
	head := q.head

	// Read next pointer
	newHead := head.next

	// Is queue empty?
	if newHead == nil {
		return nil, false // Queue was empty
	}

	// Queue not empty, read value before release
	value := newHead.value

	// Swing Head to next node
	q.head = newHead

	// In Go, we don't need to free the node explicitly
	// as the garbage collector will handle it

	return value, true // Queue was not empty, dequeue succeeded
}

// Peek returns the value at the front of the queue without removing it
// Returns (nil, false) if the queue is empty
func (q *TwoLockQueue) Peek() (interface{}, bool) {
	// Acquire the head lock to access Head
	q.headLock.Lock()
	defer q.headLock.Unlock()

	// Read Head
	head := q.head

	// Read next pointer
	newHead := head.next

	// Is queue empty?
	if newHead == nil {
		return nil, false // Queue was empty
	}

	// Queue not empty, return the value without modifying queue
	return newHead.value, true
}

// IsEmpty returns true if the queue is empty
func (q *TwoLockQueue) IsEmpty() bool {
	q.headLock.Lock()
	defer q.headLock.Unlock()

	return q.head.next == nil
}
