package queue

import (
	"sync"
	"testing"
	"time"
)

func TestNewQueue(t *testing.T) {
	q := NewQueue(10)
	if q == nil {
		t.Fatal("NewQueue returned nil")
	}
	if q.items == nil {
		t.Error("queue items channel is nil")
	}
	if q.done == nil {
		t.Error("queue done channel is nil")
	}
	if cap(q.items) != 10 {
		t.Errorf("expected capacity 10, got %d", cap(q.items))
	}
	if !q.IsEmpty() {
		t.Error("new queue should be empty")
	}
}

func TestEnqueueDequeue(t *testing.T) {
	q := NewQueue(10)

	// Test enqueue
	success := q.Enqueue(42)
	if !success {
		t.Error("Enqueue should return true for successful enqueue")
	}

	// Check queue size
	if q.Size() != 1 {
		t.Errorf("expected size 1, got %d", q.Size())
	}

	// Test dequeue
	value, ok := q.Dequeue()
	if !ok {
		t.Error("Dequeue failed, queue should have an item")
	}
	if value != 42 {
		t.Errorf("expected 42, got %v", value)
	}

	// Queue should be empty again
	if !q.IsEmpty() {
		t.Error("queue should be empty after dequeue")
	}

	// Dequeue from empty queue should block, so use TryDequeue
	value, ok = q.TryDequeue()
	if ok {
		t.Error("TryDequeue from empty queue should return ok=false")
	}
	if value != nil {
		t.Errorf("TryDequeue from empty queue should return nil, got %v", value)
	}
}

func TestTryEnqueueTryDequeue(t *testing.T) {
	// Create a queue with capacity 1
	q := NewQueue(1)

	// First enqueue should succeed
	success := q.TryEnqueue(1)
	if !success {
		t.Error("First TryEnqueue should succeed")
	}

	// Second enqueue should fail (queue full)
	success = q.TryEnqueue(2)
	if success {
		t.Error("Second TryEnqueue should fail when queue is full")
	}

	// Dequeue should succeed
	value, ok := q.TryDequeue()
	if !ok {
		t.Error("TryDequeue should succeed")
	}
	if value != 1 {
		t.Errorf("Expected value 1, got %v", value)
	}

	// Second dequeue should fail (queue empty)
	_, ok = q.TryDequeue()
	if ok {
		t.Error("TryDequeue from empty queue should fail")
	}
}

func TestDequeueBlocking(t *testing.T) {
	q := NewQueue(1)

	// Start a goroutine that will wait for an item
	var wg sync.WaitGroup
	wg.Add(1)

	var result interface{}
	var success bool

	go func() {
		defer wg.Done()
		// This should block until an item is available
		result, success = q.Dequeue()
	}()

	// Give the goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Add an item to the queue
	q.Enqueue(123)

	// Wait for the goroutine to process the item
	wg.Wait()

	// Check results
	if !success {
		t.Error("Dequeue should have succeeded")
	}
	if result != 123 {
		t.Errorf("expected 123, got %v", result)
	}
}

func TestDequeueWithClose(t *testing.T) {
	q := NewQueue(1)

	// Start a goroutine that will wait for an item
	var wg sync.WaitGroup
	wg.Add(1)

	var result interface{}
	var success bool

	go func() {
		defer wg.Done()
		// This should block until the queue is closed
		result, success = q.Dequeue()
	}()

	// Give the goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Close the queue
	q.Close()

	// Wait for the goroutine to receive the signal
	wg.Wait()

	// Check results
	if success {
		t.Error("Dequeue should have failed on closed queue")
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestClose(t *testing.T) {
	q := NewQueue(10)

	// Enqueue an item
	q.Enqueue(1)

	// Close the queue
	q.Close()

	// Should still be able to dequeue existing items
	value, ok := q.Dequeue()
	if !ok {
		t.Error("Should be able to dequeue existing items after close")
	}
	if value != 1 {
		t.Errorf("Expected 1, got %v", value)
	}

	// Cannot enqueue after close
	success := q.Enqueue(2)
	if success {
		t.Error("Should not be able to enqueue after close")
	}

	// TryEnqueue should also fail
	success = q.TryEnqueue(3)
	if success {
		t.Error("TryEnqueue should fail after close")
	}

	// Dequeue should now return false (queue empty and closed)
	_, ok = q.Dequeue()
	if ok {
		t.Error("Dequeue should return false when queue is empty and closed")
	}

	// Verify the queue is marked as closed
	if !q.IsClosed() {
		t.Error("IsClosed should return true for closed queue")
	}
}

func TestProcess(t *testing.T) {
	q := NewQueue(10)

	processed := make([]int, 0, 5)
	var processMutex sync.Mutex

	// Start processing
	done := q.Process(func(item any) error {
		val := item.(int)
		processMutex.Lock()
		processed = append(processed, val)
		processMutex.Unlock()
		return nil
	})

	// Add items
	for i := 1; i <= 5; i++ {
		q.Enqueue(i)
	}

	// Wait a bit for processing
	time.Sleep(100 * time.Millisecond)

	// Close queue and wait for processor to finish
	q.Close()
	<-done

	// Check results
	processMutex.Lock()
	defer processMutex.Unlock()

	if len(processed) != 5 {
		t.Errorf("expected 5 processed items, got %d", len(processed))
	}

	// Check order
	for i := 0; i < len(processed); i++ {
		if processed[i] != i+1 {
			t.Errorf("wrong processing order: item %d should be %d, got %d", i, i+1, processed[i])
		}
	}
}

func TestConcurrentEnqueueDequeue(t *testing.T) {
	q := NewQueue(1000)
	const numProducers = 4
	const numConsumers = 4
	const itemsPerProducer = 100

	// Channel to collect all dequeued items
	results := make(chan int, numProducers*itemsPerProducer)

	// Start consumers
	var wg sync.WaitGroup
	wg.Add(numConsumers)
	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			for {
				item, ok := q.Dequeue()
				if !ok {
					return // Queue closed
				}
				results <- item.(int)
			}
		}()
	}

	// Start producers
	var producerWg sync.WaitGroup
	producerWg.Add(numProducers)
	for p := 0; p < numProducers; p++ {
		go func(producerID int) {
			defer producerWg.Done()
			base := producerID * itemsPerProducer
			for i := 0; i < itemsPerProducer; i++ {
				success := q.Enqueue(base + i)
				if !success {
					t.Errorf("Enqueue failed for item %d", base+i)
				}
				// Add small delay to reduce contention
				if i%10 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(p)
	}

	// Wait for producers to finish
	producerWg.Wait()

	// Close the queue to signal consumers to stop
	q.Close()

	// Wait for consumers to finish
	wg.Wait()
	close(results)

	// Count the results
	seen := make(map[int]bool)
	for item := range results {
		seen[item] = true
	}

	// Verify all items were processed
	if len(seen) != numProducers*itemsPerProducer {
		t.Errorf("expected %d unique items, got %d", numProducers*itemsPerProducer, len(seen))
	}

	// Verify each expected item was seen
	for p := 0; p < numProducers; p++ {
		base := p * itemsPerProducer
		for i := 0; i < itemsPerProducer; i++ {
			if !seen[base+i] {
				t.Errorf("item %d was not processed", base+i)
				// Only report a few missing items to avoid flooding the output
				if len(seen) < numProducers*itemsPerProducer-10 {
					break
				}
			}
		}
	}
}

func TestResourceQueueManager(t *testing.T) {
	manager := NewResourceQueueManager()

	// Get queues for different resources
	q1 := manager.GetOrCreateQueue("app1", "v1", "resource1", 10)
	q2 := manager.GetOrCreateQueue("app1", "v1", "resource2", 10)
	q3 := manager.GetOrCreateQueue("app2", "v1", "resource1", 10)

	// Should get the same queue instance for the same app/version/resource
	q1Again := manager.GetOrCreateQueue("app1", "v1", "resource1", 10)
	if q1 != q1Again {
		t.Error("GetOrCreateQueue should return the same queue instance for the same app/version/resource")
	}

	// Different resources should get different queues
	if q1 == q2 || q1 == q3 || q2 == q3 {
		t.Error("different resources should get different queue instances")
	}

	// Test enqueueing to specific queues
	q1.Enqueue("item1")
	q2.Enqueue("item2")
	q3.Enqueue("item3")

	// Check that items went to the right queues
	item1, ok1 := q1.Dequeue()
	item2, ok2 := q2.Dequeue()
	item3, ok3 := q3.Dequeue()

	if !ok1 || item1 != "item1" {
		t.Errorf("expected 'item1', got %v (ok=%v)", item1, ok1)
	}
	if !ok2 || item2 != "item2" {
		t.Errorf("expected 'item2', got %v (ok=%v)", item2, ok2)
	}
	if !ok3 || item3 != "item3" {
		t.Errorf("expected 'item3', got %v (ok=%v)", item3, ok3)
	}

	// Test getting a queue
	q1ByGet := manager.GetQueue("app1", "v1", "resource1")
	if q1ByGet != q1 {
		t.Error("GetQueue should return the same queue as GetOrCreateQueue")
	}

	// Test closing a specific queue
	manager.CloseQueue("app1", "v1", "resource1")

	// The queue should be closed
	if !q1.IsClosed() {
		t.Error("Queue should be closed after CloseQueue")
	}

	success := q1.Enqueue("should fail")
	if success {
		t.Error("Enqueue should fail on closed queue")
	}

	// Other queues should still be open
	if q2.IsClosed() {
		t.Error("Other queues should not be closed")
	}

	success = q2.Enqueue("should succeed")
	if !success {
		t.Error("Enqueue should succeed on other queues")
	}

	// Test closing all queues
	manager.Close()

	// All queues should be closed
	if !q1.IsClosed() || !q2.IsClosed() || !q3.IsClosed() {
		t.Error("All queues should be closed after Close")
	}

	// After closing the manager, GetOrCreateQueue should return nil
	q4 := manager.GetOrCreateQueue("app3", "v1", "resource1", 10)
	if q4 != nil {
		t.Error("GetOrCreateQueue should return nil after manager is closed")
	}
}
