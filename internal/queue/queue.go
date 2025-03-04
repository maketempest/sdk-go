package queue

import (
	"sync"
)

// Queue is a thread-safe queue implemented using Go channels.
// It provides FIFO (First In, First Out) semantics.
type Queue struct {
	items chan interface{}
	done  chan struct{}
	mutex sync.RWMutex // Protects access to done channel and for len/cap operations
}

// NewQueue creates a new queue with the specified capacity.
// If capacity is 0, the queue will be unbuffered.
func NewQueue(capacity int) *Queue {
	return &Queue{
		items: make(chan interface{}, capacity),
		done:  make(chan struct{}),
	}
}

// Enqueue adds an item to the queue.
// It returns false if the queue is closed, true otherwise.
// This function will block if the queue is at capacity.
func (q *Queue) Enqueue(item interface{}) bool {
	// Fast path: check if done channel is closed
	select {
	case <-q.done:
		return false
	default:
		// Continue with enqueue
	}

	// Try to enqueue the item
	select {
	case q.items <- item:
		return true
	case <-q.done:
		return false
	}
}

// TryEnqueue attempts to add an item to the queue without blocking.
// It returns true if the item was enqueued, false if the queue was full or closed.
func (q *Queue) TryEnqueue(item interface{}) bool {
	// Fast path: check if done channel is closed
	select {
	case <-q.done:
		return false
	default:
		// Continue with enqueue
	}

	// Try to enqueue without blocking
	select {
	case q.items <- item:
		return true
	case <-q.done:
		return false
	default:
		return false
	}
}

// Dequeue removes and returns an item from the queue.
// It blocks until an item is available or the queue is closed.
// Returns the dequeued item and a boolean indicating success.
func (q *Queue) Dequeue() (interface{}, bool) {
	// Try to dequeue an item
	select {
	case item := <-q.items:
		return item, true
	case <-q.done:
		// Queue is closed, but check if there are remaining items
		select {
		case item := <-q.items:
			return item, true
		default:
			return nil, false
		}
	}
}

// TryDequeue attempts to remove and return an item from the queue without blocking.
// Returns the dequeued item and a boolean indicating success.
func (q *Queue) TryDequeue() (interface{}, bool) {
	select {
	case item := <-q.items:
		return item, true
	case <-q.done:
		// Queue is closed, but check if there are remaining items
		select {
		case item := <-q.items:
			return item, true
		default:
			return nil, false
		}
	default:
		return nil, false
	}
}

// Close closes the queue, preventing new items from being enqueued.
// Existing items can still be dequeued.
func (q *Queue) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	select {
	case <-q.done:
		// Already closed
	default:
		close(q.done)
	}
}

// Size returns the current number of items in the queue.
// Note: This is only a snapshot - the size may change immediately after this call.
func (q *Queue) Size() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.items)
}

// Capacity returns the capacity of the queue.
func (q *Queue) Capacity() int {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return cap(q.items)
}

// IsEmpty returns true if the queue is empty.
// Note: This is only a snapshot - the state may change immediately after this call.
func (q *Queue) IsEmpty() bool {
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	return len(q.items) == 0
}

// IsClosed returns true if the queue has been closed.
func (q *Queue) IsClosed() bool {
	select {
	case <-q.done:
		return true
	default:
		return false
	}
}

// Process starts processing items from the queue using the provided function.
// It returns a channel that is closed when processing completes.
// Processing completes when the queue is closed and all items have been processed.
func (q *Queue) Process(processor func(interface{}) error) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)

		for {
			item, ok := q.Dequeue()
			if !ok {
				return // Queue is closed and empty
			}

			// Process the item - ignore errors (caller should handle in processor function)
			_ = processor(item)
		}
	}()
	return done
}

// ResourceQueueManager manages queues for different resources.
// It ensures operations for the same resource are processed sequentially,
// while allowing operations for different resources to be processed concurrently.
type ResourceQueueManager struct {
	queues map[string]*Queue // Maps "appName/resourceID" to a queue
	mutex  sync.RWMutex
	closed bool
}

// NewResourceQueueManager creates a new manager for resource queues.
func NewResourceQueueManager() *ResourceQueueManager {
	return &ResourceQueueManager{
		queues: make(map[string]*Queue),
		closed: false,
	}
}

// GetOrCreateQueue gets an existing queue or creates a new one if it doesn't exist.
// If the manager is closed, this returns nil.
func (m *ResourceQueueManager) GetOrCreateQueue(appName, resourceID string, capacity int) *Queue {
	key := appName + "/" + resourceID

	// Try read-only first for performance
	m.mutex.RLock()
	if m.closed {
		m.mutex.RUnlock()
		return nil
	}

	q, exists := m.queues[key]
	m.mutex.RUnlock()

	if exists {
		return q
	}

	// Need write lock to create a new queue
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check again in case another goroutine created it or the manager was closed
	if m.closed {
		return nil
	}

	q, exists = m.queues[key]
	if exists {
		return q
	}

	// Create a new queue
	q = NewQueue(capacity)
	m.queues[key] = q
	return q
}

// GetQueue gets an existing queue, or returns nil if it doesn't exist.
func (m *ResourceQueueManager) GetQueue(appName, resourceID string) *Queue {
	key := appName + "/" + resourceID

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.closed {
		return nil
	}

	return m.queues[key]
}

// Close closes all queues managed by this manager and prevents new queues from being created.
func (m *ResourceQueueManager) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return
	}

	m.closed = true
	for _, q := range m.queues {
		q.Close()
	}
}

// CloseQueue closes a specific queue managed by this manager.
func (m *ResourceQueueManager) CloseQueue(appName, resourceID string) {
	key := appName + "/" + resourceID

	m.mutex.RLock()
	q, exists := m.queues[key]
	m.mutex.RUnlock()

	if exists {
		q.Close()
	}
}
