package queue

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewTwoLockQueue(t *testing.T) {
	q := NewTwoLockQueue()
	if q == nil {
		t.Fatal("NewTwoLockQueue returned nil")
	}
	if q.head == nil {
		t.Error("queue head is nil")
	}
	if q.tail == nil {
		t.Error("queue tail is nil")
	}
	if q.head != q.tail {
		t.Error("head and tail should point to the same dummy node in an empty queue")
	}
	if q.head.value != nil {
		t.Error("dummy node value should be nil")
	}
	if q.head.next != nil {
		t.Error("dummy node next should be nil")
	}
}

func TestEnqueueDequeue(t *testing.T) {
	q := NewTwoLockQueue()

	// Test enqueue
	q.Enqueue(42)

	// Check if queue is not empty
	if q.IsEmpty() {
		t.Error("queue should not be empty after enqueue")
	}

	// Test dequeue
	value, ok := q.Dequeue()
	if !ok {
		t.Error("dequeue failed, but queue should have an item")
	}
	if value != 42 {
		t.Errorf("expected 42, got %v", value)
	}

	// Queue should be empty again
	if !q.IsEmpty() {
		t.Error("queue should be empty after dequeue")
	}

	// Dequeue from empty queue should return (nil, false)
	value, ok = q.Dequeue()
	if ok {
		t.Error("dequeue from empty queue should return ok=false")
	}
	if value != nil {
		t.Errorf("dequeue from empty queue should return nil, got %v", value)
	}
}

func TestPeek(t *testing.T) {
	q := NewTwoLockQueue()

	// Peek on empty queue
	value, ok := q.Peek()
	if ok {
		t.Error("peek on empty queue should return ok=false")
	}
	if value != nil {
		t.Errorf("peek on empty queue should return nil, got %v", value)
	}

	// Add item and peek
	q.Enqueue(100)
	value, ok = q.Peek()
	if !ok {
		t.Error("peek should return ok=true for non-empty queue")
	}
	if value != 100 {
		t.Errorf("expected 100, got %v", value)
	}

	// Peek should not remove the item
	if q.IsEmpty() {
		t.Error("queue should not be empty after peek")
	}

	// Dequeue should still work after peek
	value, ok = q.Dequeue()
	if !ok {
		t.Error("dequeue failed after peek")
	}
	if value != 100 {
		t.Errorf("expected 100, got %v", value)
	}
}

func TestIsEmpty(t *testing.T) {
	q := NewTwoLockQueue()

	// New queue should be empty
	if !q.IsEmpty() {
		t.Error("new queue should be empty")
	}

	// Queue with item should not be empty
	q.Enqueue(1)
	if q.IsEmpty() {
		t.Error("queue with item should not be empty")
	}

	// Queue should be empty after removing the item
	q.Dequeue()
	if !q.IsEmpty() {
		t.Error("queue should be empty after removing all items")
	}
}

func TestMultipleEnqueueDequeue(t *testing.T) {
	q := NewTwoLockQueue()
	items := []int{1, 2, 3, 4, 5}

	// Enqueue items
	for _, item := range items {
		q.Enqueue(item)
	}

	// Dequeue items and check order
	for _, expected := range items {
		value, ok := q.Dequeue()
		if !ok {
			t.Fatalf("dequeue failed, expected more items")
		}
		if value != expected {
			t.Errorf("expected %v, got %v", expected, value)
		}
	}

	// Queue should be empty
	if !q.IsEmpty() {
		t.Error("queue should be empty after dequeueing all items")
	}
}

func TestConcurrentEnqueueDequeue(t *testing.T) {
	q := NewTwoLockQueue()
	const numProducers = 4
	const numConsumers = 4
	const itemsPerProducer = 1000

	// Channel to collect all dequeued items
	results := make(chan interface{}, numProducers*itemsPerProducer)

	// Wait group to synchronize all goroutines
	var wg sync.WaitGroup

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < itemsPerProducer; i++ {
				// Create unique values with producer ID and item number
				value := fmt.Sprintf("p%d-i%d", producerID, i)
				q.Enqueue(value)
			}
		}(p)
	}

	// Give producers a head start
	time.Sleep(10 * time.Millisecond)

	// Track how many items were successfully dequeued
	var dequeued int64

	// Start consumers
	for c := 0; c < numConsumers; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				value, ok := q.Dequeue()
				if !ok {
					// No item available, check if producers are done
					if dequeued >= int64(numProducers*itemsPerProducer) {
						return
					}
					// Wait a bit and try again
					time.Sleep(time.Millisecond)
					continue
				}

				// Successfully dequeued an item
				results <- value
				dequeued++

				// Check if we're done
				if dequeued >= int64(numProducers*itemsPerProducer) {
					return
				}
			}
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()
	close(results)

	// Count results
	count := 0
	for range results {
		count++
	}

	// Ensure we got all items
	if count != numProducers*itemsPerProducer {
		t.Errorf("expected %d items, got %d", numProducers*itemsPerProducer, count)
	}
}

func TestConcurrentPeekEnqueueDequeue(t *testing.T) {
	q := NewTwoLockQueue()

	// Add initial item
	q.Enqueue("initial")

	// Parallel operations
	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: Peek repeatedly
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			value, ok := q.Peek()
			if ok && value == nil {
				t.Error("peek returned nil for non-empty queue")
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Goroutine 2: Enqueue items
	go func() {
		defer wg.Done()
		for i := range 100 {
			q.Enqueue(i)
			time.Sleep(time.Millisecond)
		}
	}()

	// Goroutine 3: Dequeue items
	go func() {
		defer wg.Done()
		for range 50 { // Dequeue fewer items than enqueued
			q.Dequeue()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	wg.Wait()

	// Queue should not be empty
	if q.IsEmpty() {
		t.Error("queue should not be empty after concurrent operations")
	}

	// Dequeue remaining items
	count := 0
	for {
		_, ok := q.Dequeue()
		if !ok {
			break
		}
		count++
	}

	// Should have dequeued at least some items
	if count == 0 {
		t.Error("expected to dequeue remaining items")
	}
}
