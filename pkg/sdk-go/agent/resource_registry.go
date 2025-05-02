package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/tempestdx/sdk-go/internal/queue"
)

// ResourceRegistry manages resource-specific queues and pollers
type ResourceRegistry struct {
	queues        *queue.ResourceQueueManager
	pollers       map[string]context.CancelFunc
	pollMutex     sync.RWMutex
	agent         *agent
	workerTracker *WorkerTracker
}

func NewResourceRegistry(a *agent) *ResourceRegistry {
	return &ResourceRegistry{
		queues:        a.resourceQueues,
		pollers:       make(map[string]context.CancelFunc),
		pollMutex:     sync.RWMutex{},
		agent:         a,
		workerTracker: a.workerTracker,
	}
}

// GetQueueForResource returns a queue for the given app/resource combination.
// If a queue doesn't exist, one is created and stored in the registry.
func (r *ResourceRegistry) GetQueueForResource(appName, version, resourceID string) *queue.Queue {
	// Create a unique key for this app/resource combination
	return r.queues.GetOrCreateQueue(appName, version, resourceID, 100)
}

// StartPoller starts a poller for the given app/resource combination if one
// doesn't already exist. Returns true if a new poller was started, false otherwise.
func (r *ResourceRegistry) StartPoller(appName, version, resourceID string) bool {
	// Create a unique key for this app/resource combination
	key := fmt.Sprintf("%s/%s/%s", appName, version, resourceID)

	// Lock to prevent race conditions when checking/modifying the pollers map
	r.pollMutex.Lock()
	defer r.pollMutex.Unlock()

	// Check if a poller is already running for this resource
	if _, exists := r.pollers[key]; exists {
		return false
	}

	// Create a cancellable context for this poller
	ctx, cancel := context.WithCancel(context.Background())
	r.pollers[key] = cancel

	// Start the resource poller goroutine
	r.workerTracker.Add(1)
	go r.agent.startResourcePoller(ctx, appName, version, resourceID)

	return true
}

// StopAll stops all resource pollers and clears the pollers map
func (r *ResourceRegistry) StopAll() {
	r.pollMutex.Lock()
	defer r.pollMutex.Unlock()

	// Cancel all poller contexts
	for key, cancel := range r.pollers {
		r.agent.logger.Debug("Stopping resource poller", "resource", key)
		cancel()
	}

	// Clear the pollers map
	r.pollers = make(map[string]context.CancelFunc)
}
