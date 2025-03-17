package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tempestdx/sdk-go/app"
	"github.com/tempestdx/sdk-go/internal/queue"
	"github.com/tempestdx/sdk-go/resource"
)

// LogFormat represents the format of logs (text or JSON)
type LogFormat int

const (
	LogFormatText LogFormat = iota
	LogFormatJSON
)

// operationMetadata stores the metadata for an operation
type operationMetadata struct {
	Name        string
	Priority    int
	Concurrency int
}

type canonicalBinding struct {
	Operations []operationMetadata
}

type resourceName string
type resourceCanonicalBindings map[resourceName]map[resource.CanonicalType][]canonicalBinding

type appConfig struct {
	*app.App
	SupportedVersions    []string
	ResourceCanonicalMap resourceCanonicalBindings
}

// Operation represents a work item to be executed
type Operation struct {
	TaskID        string                     `json:"task_id"`
	AppName       string                     `json:"app_name"`
	Version       string                     `json:"version"`
	ResourceID    string                     `json:"resource_id"`
	OperationName string                     `json:"operation_name"`
	Request       *resource.OperationRequest `json:"request"`
}

// OperationResult represents the result of an executed operation
type OperationResult struct {
	TaskID    string         `json:"task_id"`
	Status    string         `json:"status"`           // "success", "failure"
	Message   string         `json:"message"`          // Human-readable message
	Error     string         `json:"error,omitempty"`  // Error details if failed
	Output    map[string]any `json:"output,omitempty"` // Operation output data
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
}

// JSON-friendly canonical binding structures
type jsonCanonicalBinding struct {
	Operations []jsonOperationMetadata `json:"operations"`
}

type jsonOperationMetadata struct {
	Name        string `json:"name"`
	Priority    int    `json:"priority"`
	Concurrency int    `json:"concurrency"`
}

// agent is the central handler for application operations
type agent struct {
	apiKey         string
	apps           map[string]*appConfig
	resourceQueues *queue.ResourceQueueManager   // Manager for per-resource operation queues
	resultQueue    *queue.Queue                  // Queue for operation results
	pollers        map[string]context.CancelFunc // Map of poller cancel functions keyed by "appName/resourceID"
	pollersMutex   sync.RWMutex                  // Mutex to protect the pollers map
	server         *http.Server
	mux            *http.ServeMux
	routes         map[string]http.Handler
	workers        int
	resultWorkers  int
	workerTracker  *WorkerTracker // Tracks worker goroutines
	resultTracker  *WorkerTracker // Tracks result reporter goroutines
	workerStop     chan struct{}
	resultStop     chan struct{}
	httpClient     *http.Client // Client for API requests
	apiURL         string       // Base URL for Tempest API
	logger         *slog.Logger // Logger for agent operations
	pollTimeout    time.Duration
	stopOptions    StopOptions // Default options for stopping the agent
	shutdownMutex  sync.Mutex  // Protects shutdown process
	isShutdown     bool        // Tracks if shutdown has been called
}

// QueueOptions configures the operation queue
type QueueOptions struct {
	NumWorkers       int           // Number of concurrent workers processing operations
	NumResultWorkers int           // Number of workers for reporting results
	PollTimeout      time.Duration // Timeout for polling operations from the queue
}

// DefaultQueueOptions provides sensible defaults for the queue
var DefaultQueueOptions = QueueOptions{
	NumWorkers:       5,
	NumResultWorkers: 2,
	PollTimeout:      100 * time.Millisecond,
}

// StopOptions configures the behavior of the Stop method
type StopOptions struct {
	NoTimeout bool          // If true, wait indefinitely for workers to finish
	Timeout   time.Duration // Maximum time to wait for workers to finish (default 5s)
}

// DefaultStopOptions provides sensible defaults for stopping the agent
var DefaultStopOptions = StopOptions{
	NoTimeout: true,
	Timeout:   5 * time.Second,
}

// New creates a new agent with the provided options
// Environment variables take precedence over options provided in code
func New(options ...Option) (*agent, error) {
	// Default API key
	apiKey := getEnv("TEMPEST_API_KEY", "")

	// Default HTTP server address
	serverAddr := getEnv("TEMPEST_SERVER_ADDR", ":8080")

	// Default Tempest API URL
	apiURL := getEnv("TEMPEST_API_URL", "https://api.tempestdx.com")

	// Default number of workers
	workers, _ := strconv.Atoi(getEnv("TEMPEST_NUM_WORKERS", "5"))

	// Default number of result workers
	resultWorkers, _ := strconv.Atoi(getEnv("TEMPEST_NUM_RESULT_WORKERS", "2"))

	// Default poll timeout
	pollTimeoutStr := getEnv("TEMPEST_POLL_TIMEOUT", "100ms")
	pollTimeout, _ := time.ParseDuration(pollTimeoutStr)

	// Create HTTP mux/server
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	// Configure logger
	var logger *slog.Logger
	logLevel := getEnv("TEMPEST_LOG_LEVEL", "info")
	level := parseLogLevel(logLevel)

	logFormat := getEnv("TEMPEST_LOG_FORMAT", "json")
	if strings.ToLower(logFormat) == "text" {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
	} else {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
	}

	// Create worker trackers
	workerTracker := &WorkerTracker{
		name:   "workers",
		logger: logger,
	}

	resultTracker := &WorkerTracker{
		name:   "result_reporters",
		logger: logger,
	}

	// Create the agent
	a := &agent{
		apiKey:         apiKey,
		apps:           make(map[string]*appConfig),
		resultQueue:    queue.NewQueue(1000),
		pollers:        make(map[string]context.CancelFunc),
		server:         server,
		mux:            mux,
		routes:         make(map[string]http.Handler),
		workers:        workers,
		resultWorkers:  resultWorkers,
		workerTracker:  workerTracker,
		resultTracker:  resultTracker,
		workerStop:     make(chan struct{}),
		resultStop:     make(chan struct{}),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		apiURL:         apiURL,
		logger:         logger,
		pollTimeout:    pollTimeout,
		stopOptions:    DefaultStopOptions,
		shutdownMutex:  sync.Mutex{},
		isShutdown:     false,
		resourceQueues: queue.NewResourceQueueManager(),
	}

	// Create resource registry
	registry := NewResourceRegistry(a)
	a.resourceQueues = registry.queues

	// Apply all options
	for _, option := range options {
		if err := option(a); err != nil {
			return nil, err
		}
	}

	// Register default handlers
	a.registerDefaultHandlers()

	return a, nil
}

// RegisterApp registers an app with the agent and sets up its HTTP routes
func (a *agent) RegisterApp(app *app.App, supportedVersions []string) {
	a.logger.Info("Registering app", "app_name", app.Name, "versions", supportedVersions)

	a.apps[app.Name] = &appConfig{
		App:               app,
		SupportedVersions: supportedVersions,
	}

	// Generate routes and canonical bindings
	a.apps[app.Name].generateCanonicalBindings()
	canonicalRoutes := a.apps[app.Name].generateCanonicalRoutes()
	operationRoutes := a.apps[app.Name].generateOperationRoutes()

	a.logger.Debug("Generated routes",
		"app_name", app.Name,
		"operation_routes", len(operationRoutes),
		"canonical_routes", len(canonicalRoutes))

	// Register HTTP handlers for operation routes with /operations/ prefix
	for path := range operationRoutes {
		// Parse elements from path
		// Expected format: appName/version/resourceID/operations/operationName
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[3] != "operations" {
			a.logger.Warn("Invalid operations path format", "path", path)
			continue // Invalid path format
		}

		appName := parts[0]
		version := parts[1]
		resourceID := parts[2]
		operationName := parts[4]

		// Create and register the operation handler with operations prefix
		a.registerHandler("operations", appName, version, resourceID, operationName)
	}

	// Register HTTP handlers for canonical routes
	for path := range canonicalRoutes {
		// Parse elements from path
		// Expected format: appName/version/resourceID/canonicalType
		parts := strings.Split(path, "/")
		if len(parts) != 4 {
			a.logger.Warn("Invalid canonical path format", "path", path)
			continue // Invalid path format
		}

		appName := parts[0]
		version := parts[1]
		resourceID := parts[2]
		canonicalTypeStr := parts[3]

		// Create and register the canonical handler
		a.registerHandler("canonical", appName, version, resourceID, canonicalTypeStr)
	}

	a.logger.Info("App registration complete", "app_name", app.Name)
}

// Run starts the agent and blocks until it receives a termination signal
// (SIGINT or SIGTERM), then performs a graceful shutdown.
// This is the main entry point for SDK users to start the agent, which will
// autonomously poll for operations, route them to appropriate queues, and
// process them according to the registered handlers. It handles all aspects
// of operation polling, processing, and result reporting without requiring
// any further interaction from the SDK user.
//
// Example usage:
//
//	agent, err := tempest.NewAgent()
//	if err != nil {
//	    log.Fatalf("Failed to create agent: %v", err)
//	}
//	agent.RegisterOperation("myapp", "v1", "postgres", "create", handleCreate)
//	// ... register other operations
//	if err := agent.Run(); err != nil {
//	    log.Fatalf("Agent terminated with error: %v", err)
//	}
func (a *agent) Run() error {
	// Start the agent in a goroutine
	go func() {
		if err := a.Start(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("Failed to start agent", "error", err)
			os.Exit(1)
		}
	}()

	a.logger.Info("Agent running, press Ctrl+C to stop")

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	a.logger.Info("Received termination signal, shutting down...", "signal", sig)

	// Gracefully shutdown the agent
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return a.Stop(ctx)
}

// Start starts the HTTP server, result reporters, and initializes the infrastructure
// for processing operations. This method is called internally by Run() and sets up
// all necessary components for the agent to autonomously poll for operations, process
// them, and report results back to the Tempest API.
//
// SDK users typically should call Run() instead, which calls Start() internally
// and also handles graceful shutdown on termination signals.
func (a *agent) Start() error {
	a.logger.Info("Starting agent", "workers", a.workers, "result_workers", a.resultWorkers)

	// Log all registered routes
	routes := make([]string, 0, len(a.routes))
	for route := range a.routes {
		routes = append(routes, route)
	}
	a.logger.Info("Registered routes", "routes", routes)

	// Initialize worker stop channel
	a.workerStop = make(chan struct{})

	// Initialize result stop channel
	a.resultStop = make(chan struct{})

	// Start worker goroutines for operation processing
	// NOTE: We no longer use global workers, each resource has its own poller
	// So we don't need to increment the worker wait group here anymore
	// We now use per-resource pollers instead of the global operationWorker
	// Each app/resource combination will have its own dedicated queue

	// Start result worker goroutines
	a.resultTracker.Add(a.resultWorkers)
	for range a.resultWorkers {
		go a.resultReporter()
	}

	// Start the API task poller to continuously poll the Tempest API
	// for new operations to process. This fetches tasks from the API and
	// routes them to the appropriate resource queues.
	registry := &ResourceRegistry{
		queues:        a.resourceQueues,
		pollers:       a.pollers,
		pollMutex:     sync.RWMutex{},
		agent:         a,
		workerTracker: a.workerTracker,
	}
	go a.taskPoller(registry)

	// Start HTTP server in a goroutine
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("Error starting HTTP server", "error", err)
		}
	}()

	a.logger.Info("Agent started", "api_url", a.apiURL)

	return nil
}

// Stop gracefully shuts down the agent using the configured stop options
func (a *agent) Stop(ctx context.Context) error {
	a.logger.Info("Stopping agent", "no_timeout", a.stopOptions.NoTimeout)

	// Prevent multiple shutdown calls
	a.shutdownMutex.Lock()
	if a.isShutdown {
		a.shutdownMutex.Unlock()
		a.logger.Info("Agent already shutting down, ignoring duplicate stop request")
		return nil
	}
	a.isShutdown = true
	a.shutdownMutex.Unlock()

	// First shut down the HTTP server with a timeout
	var serverTimeout time.Duration
	if a.stopOptions.NoTimeout {
		serverTimeout = 30 * time.Second // Still use a reasonable timeout for HTTP server
	} else {
		serverTimeout = a.stopOptions.Timeout
	}

	serverCtx, serverCancel := context.WithTimeout(ctx, serverTimeout)
	defer serverCancel()

	if err := a.server.Shutdown(serverCtx); err != nil {
		a.logger.Error("Error shutting down HTTP server", "error", err)
	}

	// Use our shutdown function which sends all the stop signals
	a.shutdown()

	// If NoTimeout is true, wait indefinitely for workers to finish
	if a.stopOptions.NoTimeout {
		// Start a watchdog goroutine to log status periodically
		done := make(chan struct{})
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					a.logger.Info("Still waiting for workers to finish")
				}
			}
		}()

		// Wait for all workers to finish
		a.workerTracker.Wait()
		a.resultTracker.Wait()

		// Stop the watchdog
		close(done)
		a.logger.Info("All workers finished gracefully")
	} else {
		// Use a timeout for waiting
		workerDone := make(chan struct{})
		go func() {
			a.workerTracker.Wait()
			a.resultTracker.Wait()
			close(workerDone)
		}()

		select {
		case <-workerDone:
			a.logger.Info("All workers finished within timeout")
		case <-time.After(a.stopOptions.Timeout):
			a.logger.Warn("Timed out waiting for workers to finish, forcing exit",
				"timeout_seconds", a.stopOptions.Timeout.Seconds())
		}
	}

	a.logger.Info("Agent stopped")
	return nil
}

// RegisterHandler registers an HTTP handler for different types of operations
func (a *agent) registerHandler(handlerType string, appName, version, resourceID, operationName string) {
	var path string

	switch handlerType {
	case "operations":
		path = fmt.Sprintf("/%s/%s/%s/operations/%s", appName, version, resourceID, operationName)
	case "canonical":
		path = fmt.Sprintf("/%s/%s/%s/canonical/%s", appName, version, resourceID, operationName)
	default:
		a.logger.Error("Unknown handler type", "type", handlerType)
		return
	}

	a.logger.Debug("Registering handler",
		"type", handlerType,
		"path", path,
		"app", appName,
		"version", version,
		"resource", resourceID,
		"operation", operationName)

	// Create appropriate handler type based on handlerType
	var handler http.Handler

	if handlerType == "canonical" {
		// For canonical handlers, we need to create the handler directly
		// since registerCanonicalHandler registers the handler itself
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Start operation execution time
			startTime := time.Now()

			a.logger.Debug("Handling canonical operation request",
				"path", r.URL.Path,
				"method", r.Method,
				"remote_addr", r.RemoteAddr)

			// Parse request and extract task ID
			opReq, taskID, err := parseOperationRequest(r)
			if err != nil {
				a.logger.Error("Failed to parse operation request",
					"error", err,
					"path", r.URL.Path)
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			requestLogger := a.logger.With(
				"task_id", taskID,
				"canonical_type", operationName,
				"resource_id", resourceID)

			requestLogger.Info("Processing canonical operation request")

			// Execute the canonical operation
			resp, err := a.executeCanonicalOperation(
				r.Context(),
				appName, resourceID, operationName,
				opReq, taskID, startTime)

			// Calculate duration
			endTime := time.Now()
			duration := endTime.Sub(startTime)

			// Create result object for reporting
			result := &OperationResult{
				TaskID:    taskID,
				StartTime: startTime,
				EndTime:   endTime,
			}

			if err != nil {
				// Operation failed
				result.Status = "failure"
				result.Error = err.Error()
				result.Message = fmt.Sprintf("Canonical operation %s failed: %v", operationName, err)

				requestLogger.Error("Canonical operation failed",
					"error", err.Error(),
					"duration_ms", duration.Milliseconds())

				// Add result to reporting queue
				a.resultQueue.Enqueue(result)

				// Return error to caller
				http.Error(w, fmt.Sprintf("operation failed: %v", err), http.StatusInternalServerError)
				return
			}

			// Operation succeeded
			result.Status = "success"
			result.Message = fmt.Sprintf("Canonical operation %s completed successfully", operationName)

			// Extract output if available
			if resp != nil && resp.Data != nil {
				result.Output = extractOutputFromResponseData(resp.Data)
			}

			// Add result to reporting queue
			a.resultQueue.Enqueue(result)

			requestLogger.Info("Canonical operation completed successfully",
				"duration_ms", duration.Milliseconds())

			sendSuccessResponse(w, resp)
		})
	} else {
		// Create operation handler
		handler = &operationHandler{
			agent:         a,
			appName:       appName,
			version:       version,
			resourceID:    resourceID,
			operationName: operationName,
		}
	}

	a.mux.Handle(path, handler)
	a.routes[path] = handler
}

// A dedicated handler type
type operationHandler struct {
	agent         *agent
	appName       string
	version       string
	resourceID    string
	operationName string
}

// ServeHTTP handles operation requests
func (h *operationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use the common parseOperationRequest function
	opReq, taskID, err := parseOperationRequest(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing request: %v", err), http.StatusBadRequest)
		return
	}

	// Start operation execution time
	startTime := time.Now()

	h.agent.logger.Debug("Handling operation request",
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr)

	requestLogger := h.agent.logger.With("task_id", taskID, "operation", h.operationName)
	requestLogger.Info("Processing operation", "resource_id", h.resourceID)

	// Find resource definition
	appConfig, ok := h.agent.apps[h.appName]
	if !ok {
		errMsg := fmt.Sprintf("app %s not found", h.appName)
		requestLogger.Error("App not found", "app_name", h.appName)
		http.Error(w, errMsg, http.StatusNotFound)
		return
	}

	resourceDef, ok := appConfig.GetResourceDefinition(h.resourceID)
	if !ok {
		errMsg := fmt.Sprintf("resource %s not found", h.resourceID)
		requestLogger.Error("Resource not found", "resource_id", h.resourceID)
		http.Error(w, errMsg, http.StatusNotFound)
		return
	}

	// Find operation
	operation, ok := resourceDef.GetOperation(h.operationName)
	if !ok {
		errMsg := fmt.Sprintf("operation %s not found", h.operationName)
		requestLogger.Error("Operation not found", "operation", h.operationName)
		http.Error(w, errMsg, http.StatusNotFound)
		return
	}

	// Execute operation
	requestLogger.Debug("Executing operation")
	resp, err := operation.Fn(r.Context(), opReq)

	// End operation execution time
	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Prepare result for reporting
	result := &OperationResult{
		TaskID:    taskID,
		StartTime: startTime,
		EndTime:   endTime,
	}

	if err != nil {
		// Operation failed
		result.Status = "failure"
		result.Error = err.Error()
		result.Message = fmt.Sprintf("Operation %s failed: %v", h.operationName, err)

		requestLogger.Error("Operation failed",
			"error", err.Error(),
			"duration_ms", duration.Milliseconds())

		// Add result to reporting queue
		h.agent.resultQueue.Enqueue(result)

		// Return error to caller
		http.Error(w, fmt.Sprintf("operation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Operation succeeded
	result.Status = "success"
	result.Message = fmt.Sprintf("Operation %s completed successfully", h.operationName)

	// Extract output if available
	if resp != nil && resp.Data != nil {
		result.Output = extractOutputFromResponseData(resp.Data)
	}

	requestLogger.Info("Operation completed successfully",
		"duration_ms", duration.Milliseconds())

	// Add result to reporting queue
	h.agent.resultQueue.Enqueue(result)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		requestLogger.Error("Failed to encode response", "error", err)
		http.Error(w, fmt.Sprintf("error encoding response: %v", err), http.StatusInternalServerError)
		return
	}
}

// Helper function to execute a canonical operation
func (a *agent) executeCanonicalOperation(
	ctx context.Context,
	appName, resourceID, canonicalTypeStr string,
	opReq *resource.OperationRequest,
	taskID string,
	startTime time.Time,
) (*resource.OperationResponse, error) {
	logger := a.logger.With(
		"task_id", taskID,
		"canonical_type", canonicalTypeStr,
		"resource_id", resourceID,
		"app_name", appName)

	logger.Debug("Executing canonical operation")

	// Find resource definition and app config
	appConfig, ok := a.apps[appName]
	if !ok {
		logger.Error("App not found")
		return nil, fmt.Errorf("app %s not found", appName)
	}

	resourceDef, ok := appConfig.GetResourceDefinition(resourceID)
	if !ok {
		logger.Error("Resource not found")
		return nil, fmt.Errorf("resource %s not found", resourceID)
	}

	// Convert string to canonical type
	canonicalType, ok := resource.StringToCanonicalType(canonicalTypeStr)
	if !ok {
		logger.Error("Invalid canonical type")
		return nil, fmt.Errorf("invalid canonical type: %s", canonicalTypeStr)
	}

	// Find canonical bindings for this resource and type
	resName := resourceName(resourceID)
	bindings, ok := appConfig.ResourceCanonicalMap[resName][canonicalType]
	if !ok || len(bindings) == 0 {
		logger.Error("No operations found for canonical type")
		return nil, fmt.Errorf("no operations found for canonical type: %s", canonicalTypeStr)
	}

	// Get operations from first binding (typically there's only one binding per type)
	operations := bindings[0].Operations
	logger.Debug("Found operations for canonical type", "count", len(operations))

	// Sort operations by priority (lowest first)
	sortOperationsByPriority(operations)

	// Execute operations in priority order, stopping at first failure
	var lastResponse *resource.OperationResponse

	for i, opMeta := range operations {
		opLogger := logger.With("operation", opMeta.Name, "priority", opMeta.Priority, "index", i)

		// Track individual operation start time
		opStartTime := time.Now()
		opLogger.Debug("Executing operation in canonical sequence")

		// Find the operation
		operation, ok := resourceDef.GetOperation(opMeta.Name)
		if !ok {
			// Report missing operation error
			opEndTime := time.Now()
			result := &OperationResult{
				TaskID:    taskID,
				Status:    "failure",
				Message:   fmt.Sprintf("Operation %s required by canonical type %s not found", opMeta.Name, canonicalTypeStr),
				Error:     fmt.Sprintf("operation %s not found", opMeta.Name),
				StartTime: opStartTime,
				EndTime:   opEndTime,
			}
			a.resultQueue.Enqueue(result)

			errMsg := fmt.Sprintf("operation %s required by canonical type %s not found", opMeta.Name, canonicalTypeStr)
			opLogger.Error("Operation not found", "error", errMsg)
			return nil, fmt.Errorf(errMsg)
		}

		// Execute operation
		resp, err := operation.Fn(ctx, opReq)
		opEndTime := time.Now()
		opDuration := opEndTime.Sub(opStartTime)

		// Report individual operation result
		opResult := &OperationResult{
			TaskID:    taskID,
			StartTime: opStartTime,
			EndTime:   opEndTime,
		}

		if err != nil {
			// Operation failed
			opResult.Status = "failure"
			opResult.Error = err.Error()
			opResult.Message = fmt.Sprintf("Operation %s in canonical type %s failed: %v",
				opMeta.Name, canonicalTypeStr, err)
			a.resultQueue.Enqueue(opResult)

			opLogger.Error("Operation in canonical sequence failed",
				"error", err.Error(),
				"duration_ms", opDuration.Milliseconds())

			// Return error for the entire canonical operation
			return nil, fmt.Errorf("canonical operation %s failed: operation %s error: %w",
				canonicalTypeStr, opMeta.Name, err)
		}

		// Operation succeeded
		opResult.Status = "success"
		opResult.Message = fmt.Sprintf("Operation %s in canonical type %s completed successfully",
			opMeta.Name, canonicalTypeStr)

		// Extract output if available
		if resp != nil && resp.Data != nil {
			opResult.Output = extractOutputFromResponseData(resp.Data)
		}

		a.resultQueue.Enqueue(opResult)
		opLogger.Info("Operation in canonical sequence completed", "duration_ms", opDuration.Milliseconds())

		// Store successful response for final return
		lastResponse = resp
	}

	// Report overall canonical operation success
	finalResult := &OperationResult{
		TaskID:    taskID,
		Status:    "success",
		Message:   fmt.Sprintf("Canonical operation %s completed successfully", canonicalTypeStr),
		StartTime: startTime,
		EndTime:   time.Now(),
	}

	// Include output from last operation
	if lastResponse != nil && lastResponse.Data != nil {
		finalResult.Output = extractOutputFromResponseData(lastResponse.Data)
	}

	a.resultQueue.Enqueue(finalResult)
	logger.Info("All operations in canonical sequence completed successfully")

	return lastResponse, nil
}

// resultReporter sends operation results to the Tempest API
func (a *agent) resultReporter() {
	defer a.resultTracker.Done()

	a.logger.Debug("Result reporter started")

	for {
		// First check if we should stop
		select {
		case <-a.resultStop:
			a.logger.Debug("Result reporter stopping due to stop signal")
			return
		default:
			// Continue processing
		}

		// Dequeue result with a timeout to allow checking for stop signal periodically
		var item any
		var ok bool

		// Use a short timeout to allow checking stop signal frequently
		select {
		case <-a.resultStop:
			a.logger.Debug("Result reporter stopping during dequeue")
			return
		case <-time.After(100 * time.Millisecond):
			// Try to dequeue an item without blocking
			item, ok = a.resultQueue.TryDequeue()
			if !ok {
				// Queue is empty, loop back and check stop signal again
				continue
			}
		}

		// Process result
		result, ok := item.(OperationResult)
		if !ok {
			// Try using the pointer type
			resultPtr, okPtr := item.(*OperationResult)
			if !okPtr || resultPtr == nil {
				// Invalid item type, skip
				a.logger.Warn("Invalid item type in result queue", "type", fmt.Sprintf("%T", item))
				continue
			}

			// Use the dereferenced value
			result = *resultPtr
		}

		// Report result to Tempest API
		reportURL := fmt.Sprintf("%s/v1/apps/operations/report", a.apiURL)

		data, err := json.Marshal(result)
		if err != nil {
			a.logger.Error("Failed to marshal operation result", "error", err, "task_id", result.TaskID)
			continue
		}

		req, err := http.NewRequest("POST", reportURL, bytes.NewBuffer(data))
		if err != nil {
			a.logger.Error("Failed to create result report request", "error", err, "task_id", result.TaskID)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+a.apiKey)

		resp, err := a.httpClient.Do(req)
		if err != nil {
			a.logger.Error("Failed to send result report", "error", err, "task_id", result.TaskID)
			continue
		}

		// Check response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			a.logger.Error("Error reporting operation result",
				"status_code", resp.StatusCode,
				"response", string(body),
				"task_id", result.TaskID)
		} else {
			a.logger.Debug("Successfully reported operation result", "task_id", result.TaskID)
		}

		resp.Body.Close()
	}
}

// generateCanonicalRoutes creates routes for canonical operations
func (ac *appConfig) generateCanonicalRoutes() map[string]string {
	routes := make(map[string]string)

	for _, r := range ac.ResourceDefinitions() {
		// Get all canonical types for this resource
		canonicalTypes := make(map[resource.CanonicalType]bool)
		for _, op := range r.Operations() {
			for _, canonicalOp := range op.CanonicalOperations() {
				canonicalTypes[canonicalOp.Type] = true
			}
		}

		// Create a route for each canonical type and version
		for canonicalType := range canonicalTypes {
			for _, version := range ac.SupportedVersions {
				// Convert CanonicalType to string name
				typeStr := canonicalType.String()
				if typeStr != "" {
					// Remove "canonical/" from the path
					routes[fmt.Sprintf("%s/%s/%s/%s", ac.Name, version, r.UniqueID(), typeStr)] =
						fmt.Sprintf("/%s/%s", ac.Name, r.UniqueID())
				}
			}
		}
	}

	return routes
}

func (ac *appConfig) generateCanonicalBindings() {
	ac.ResourceCanonicalMap = make(resourceCanonicalBindings)

	for _, r := range ac.ResourceDefinitions() {
		resName := resourceName(r.UniqueID())
		ac.ResourceCanonicalMap[resName] = make(map[resource.CanonicalType][]canonicalBinding)

		// Group operations by canonical type
		opsByCanonicalType := make(map[resource.CanonicalType][]operationMetadata)

		for _, op := range r.Operations() {
			for _, canonicalOp := range op.CanonicalOperations() {
				opMetadata := operationMetadata{
					Name:        op.Name(),
					Priority:    canonicalOp.Priority,
					Concurrency: canonicalOp.Concurrency,
				}

				opsByCanonicalType[canonicalOp.Type] = append(
					opsByCanonicalType[canonicalOp.Type],
					opMetadata,
				)
			}
		}

		// Create canonical bindings for each type
		for canonicalType, operations := range opsByCanonicalType {
			ac.ResourceCanonicalMap[resName][canonicalType] = []canonicalBinding{
				{Operations: operations},
			}
		}
	}
}

// sortOperationsByPriority sorts operations in ascending order of priority
func sortOperationsByPriority(operations []operationMetadata) {
	sort.Slice(operations, func(i, j int) bool {
		// Lower priority comes first
		return operations[i].Priority < operations[j].Priority
	})
}

// Helper function to parse operation requests
func parseOperationRequest(r *http.Request) (*resource.OperationRequest, string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading request: %v", err)
	}
	defer r.Body.Close()

	// Parse request into a raw map first to support both formats
	var rawData map[string]any
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, "", fmt.Errorf("error parsing request: %v", err)
	}

	// Create operation request
	opReq := &resource.OperationRequest{
		Metadata: &resource.Metadata{},
	}

	// Check if this is the standard API format with nested "task" structure
	if taskRaw, hasTask := rawData["task"]; hasTask {
		task, ok := taskRaw.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("task field is not a valid object")
		}

		// Map task.input to Args
		if input, hasInput := task["input"].(map[string]any); hasInput {
			opReq.Args = input
		}

		// Map task.resource to Resource
		if resourceRaw, hasResource := task["resource"].(map[string]any); hasResource {
			// Create new resource instance
			res := &resource.Resource{}

			// Map basic string fields
			if id, ok := resourceRaw["id"].(string); ok {
				res.ExternalID = id
			}
			if name, ok := resourceRaw["name"].(string); ok {
				res.Name = name
			}
			if displayName, ok := resourceRaw["display_name"].(string); ok {
				res.DisplayName = displayName
			}
			if resType, ok := resourceRaw["type"].(string); ok {
				res.Category = resource.Category(resType)
			}

			// Map properties
			if props, ok := resourceRaw["properties"].(map[string]any); ok {
				res.Properties = props
			}

			// Add resource to request
			opReq.Resource = res
		}

		// Map task.environment_variables (if supported by OperationRequest)
		if envVarsRaw, hasEnvVars := task["environment_variables"].([]any); hasEnvVars {
			// Check if your OperationRequest has a field for environment variables
			// If so, map them here
			envVars := make(map[string]resource.EnvironmentVariable)
			for _, envVar := range envVarsRaw {
				if envVarMap, ok := envVar.(map[string]any); ok {
					key := envVarMap["key"].(string)
					value := envVarMap["value"].(string)
					envVars[key] = resource.EnvironmentVariable{
						Key:   key,
						Value: value,
					}
				}
			}
			opReq.Environment = envVars
		}

		// Map task.metadata
		if metadataRaw, hasMetadata := task["metadata"].(map[string]any); hasMetadata {
			// Map relevant metadata fields
			if taskID, ok := metadataRaw["task_id"].(string); ok {
				opReq.Metadata.TaskID = taskID
			}
		}
	} else {
		// Direct format - treat the entire body as Args/input
		opReq.Args = rawData
	}

	// Extract task_id from top level
	if taskID, ok := rawData["task_id"].(string); ok {
		opReq.Metadata.TaskID = taskID
	}

	// Map top-level metadata (might contain additional context)
	if metadataRaw, hasMetadata := rawData["metadata"].(map[string]any); hasMetadata {
		opReq.Metadata.Owners = []resource.Owner{}
		// Extract author info
		// TODO: Expand owner support to include all owners supported on server
		if authorRaw, hasAuthor := metadataRaw["author"].(map[string]any); hasAuthor {
			opReq.Metadata.Owners = append(opReq.Metadata.Owners, resource.Owner{
				Email: authorRaw["email"].(string),
				Name:  authorRaw["name"].(string),
				Type:  resource.OwnerTypeUser,
			})
		}

		// Extract project info
		if projectID, ok := metadataRaw["project_id"].(string); ok {
			opReq.Metadata.ProjectID = projectID
		}
		if projectName, ok := metadataRaw["project_name"].(string); ok {
			opReq.Metadata.ProjectName = projectName
		}
	}

	// Extract task ID for return value
	taskID := ""
	if opReq.Metadata != nil {
		taskID = opReq.Metadata.TaskID
	}

	return opReq, taskID, nil
}

// Helper function to send success response to client
func sendSuccessResponse(w http.ResponseWriter, resp *resource.OperationResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("error encoding response: %v", err), http.StatusInternalServerError)
	}
}

// generateOperationRoutes creates routes for operations with the /operations/ path format
func (ac *appConfig) generateOperationRoutes() map[string]string {
	routes := make(map[string]string)

	for _, r := range ac.ResourceDefinitions() {
		for _, op := range r.Operations() {
			// generate a route per operation, resource, and supported version
			for _, version := range ac.SupportedVersions {
				routes[fmt.Sprintf("%s/%s/%s/operations/%s", ac.Name, version, r.UniqueID(), op.Name())] =
					fmt.Sprintf("/%s/%s", ac.Name, r.UniqueID())
			}
		}
	}

	return routes
}

// shutdown is the main function for graceful agent shutdown
func (a *agent) shutdown() {
	a.logger.Info("Shutting down agent")

	// 1. Stop accepting new work by stopping pollers
	a.stopAllPollers()

	// 2. Signal worker goroutines to stop
	close(a.workerStop)

	// 3. Close resource queues to allow ongoing operations to complete
	a.closeResourceQueues()

	// 4. Allow in-flight operations to finish
	time.Sleep(100 * time.Millisecond)

	// 5. Signal result reporters to stop
	close(a.resultStop)

	// 6. Close the result queue last
	a.closeResultQueue()

	a.logger.Info("Agent shutdown sequence complete")
}

// stopAllPollers cancels all resource pollers to stop pulling operations from queues
func (a *agent) stopAllPollers() {
	a.logger.Debug("Stopping all resource pollers")

	a.pollersMutex.Lock()
	defer a.pollersMutex.Unlock()

	for key, cancel := range a.pollers {
		a.logger.Debug("Stopping poller", "key", key)
		cancel()
	}

	// Clear the pollers map
	a.pollers = make(map[string]context.CancelFunc)
	a.logger.Debug("All pollers stopped")
}

// closeResourceQueues closes all resource queues to unblock waiting operations
func (a *agent) closeResourceQueues() {
	a.logger.Debug("Closing all resource queues")

	if a.resourceQueues != nil {
		a.resourceQueues.Close()
		a.logger.Debug("All resource queues closed")
	} else {
		a.logger.Debug("No resource queues to close")
	}
}

// closeResultQueue closes the result queue after all workers have stopped
func (a *agent) closeResultQueue() {
	a.logger.Debug("Closing result queue")

	if a.resultQueue != nil {
		a.resultQueue.Close()
		a.logger.Debug("Result queue closed")
	} else {
		a.logger.Debug("No result queue to close")
	}
}

// taskPoller periodically polls the Tempest API for new tasks
// and enqueues them to the appropriate resource queues
func (a *agent) taskPoller(registry *ResourceRegistry) {
	// Add to worker tracker for shutdown management
	a.workerTracker.Add(1)
	defer a.workerTracker.Done()

	logger := a.logger.With("component", "task_poller")
	logger.Info("Starting Tempest API poller")

	ticker := time.NewTicker(a.pollTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-a.workerStop:
			logger.Info("Task poller stopping")
			return
		case <-ticker.C:
			// Poll for new tasks
			if err := a.pollAndProcessTasks(registry); err != nil {
				logger.Error("Error polling for tasks", "error", err)
				// Continue polling even on error
			}
		}
	}
}

// pollAndProcessTasks performs one polling cycle to the Tempest API
func (a *agent) pollAndProcessTasks(registry *ResourceRegistry) error {
	logger := a.logger.With("component", "task_poller")

	// Skip if no API key is configured
	if a.apiKey == "" {
		return nil
	}

	// Create request context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build the request URL
	url := fmt.Sprintf("%s/api/v1/tasks", a.apiURL)

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-200 status: %d - %s", resp.StatusCode, string(body))
	}

	// Parse the response body
	var tasks []Operation
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Debug("Received tasks from API", "count", len(tasks))

	// Process each task by enqueueing it to the appropriate resource queue
	for _, task := range tasks {
		if err := a.enqueueTaskToResourceQueue(registry, &task); err != nil {
			logger.Error("Failed to enqueue task",
				"task_id", task.TaskID,
				"app", task.AppName,
				"resource", task.ResourceID,
				"operation", task.OperationName,
				"error", err)
			// Continue processing other tasks
		}
	}

	return nil
}

// enqueueTaskToResourceQueue enqueues a task to its appropriate resource queue
func (a *agent) enqueueTaskToResourceQueue(registry *ResourceRegistry, task *Operation) error {
	logger := a.logger.With(
		"task_id", task.TaskID,
		"app", task.AppName,
		"resource", task.ResourceID,
		"operation", task.OperationName)

	logger.Debug("Enqueueing task to resource queue")

	// Get or create a queue for this resource
	queue := registry.GetQueueForResource(task.AppName, task.Version, task.ResourceID)

	// Enqueue the task
	if !queue.Enqueue(task) {
		return fmt.Errorf("failed to enqueue task (queue may be closed)")
	}

	// Start a poller for this resource if not already running
	pollerStarted := registry.StartPoller(task.AppName, task.Version, task.ResourceID)
	if pollerStarted {
		logger.Debug("Started new resource poller for incoming task")
	}

	return nil
}

// registerDefaultHandlers registers the default HTTP handlers for the agent
func (a *agent) registerDefaultHandlers() {
	// Register the describe route
	a.registerDescribeHandler()
}

// HTTP error helper
func (a *agent) httpError(w http.ResponseWriter, err error, status int, msg string, args ...any) {
	logger := a.logger.With("status", status)
	logger.Error(msg, append([]any{"error", err}, args...)...)
	http.Error(w, err.Error(), status)
}

// Context-aware error helper
func (a *agent) ctxError(ctx context.Context, err error, msg string, args ...any) error {
	logger := a.logger
	if ctx.Value("logger") != nil {
		logger = ctx.Value("logger").(*slog.Logger)
	}
	logger.Error(msg, append([]any{"error", err}, args...)...)
	return fmt.Errorf("%s: %w", msg, err)
}

// startResourcePoller is a worker that processes operations for a specific resource
func (a *agent) startResourcePoller(ctx context.Context, appName, version, resourceID string) {
	defer a.workerTracker.Done()

	logger := a.logger.With(
		"component", "resource_poller",
		"app", appName,
		"resource", resourceID)

	logger.Debug("Starting resource poller")

	// Get the queue for this resource
	queue := a.resourceQueues.GetOrCreateQueue(appName, version, resourceID, 100)
	if queue == nil {
		logger.Error("Could not create queue for resource, cannot start poller")
		return
	}

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Resource poller stopping (context canceled)")
			return
		case <-a.workerStop:
			logger.Debug("Resource poller stopping (agent shutting down)")
			return
		default:
			// Try to dequeue an operation
			opIface, ok := queue.Dequeue()
			if !ok {
				// Queue is closed or empty, check if we should exit
				select {
				case <-ctx.Done():
					return
				case <-a.workerStop:
					return
				default:
					// Queue might be temporarily empty, wait before retrying
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			// Type assertion to get the actual operation
			op, ok := opIface.(*Operation)
			if !ok {
				logger.Error("Invalid operation type from queue",
					"type", fmt.Sprintf("%T", opIface))
				continue
			}

			// Process the operation
			logger.Debug("Processing operation",
				"task_id", op.TaskID,
				"operation", op.OperationName)

			go a.processOperation(op)
		}
	}
}

// parseLogLevel converts a string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// processOperation handles the processing of a single operation
func (a *agent) processOperation(op *Operation) {
	a.workerTracker.Add(1)
	defer a.workerTracker.Done()

	// Find the app configuration
	_, ok := a.apps[op.AppName]
	if !ok {
		a.logger.Error("App not found", "app", op.AppName)
		// Report error result
		a.reportOperationError(op, "App not found")
		return
	}

	logger := a.logger.With(
		"task_id", op.TaskID,
		"app", op.AppName,
		"resource", op.ResourceID,
		"operation", op.OperationName)

	logger.Debug("Processing operation")

	// Execute the operation by looking up the appropriate handler
	// This uses existing functionality in the agent to find and execute operations
	result := a.executeOperation(op)

	// Queue result for reporting
	if !a.resultQueue.Enqueue(result) {
		logger.Error("Failed to enqueue operation result (queue might be closed)")
	}
}

// reportOperationError is a helper method to report an error for an operation
func (a *agent) reportOperationError(op *Operation, errMsg string) {
	result := &OperationResult{
		TaskID:    op.TaskID,
		Status:    "failure",
		Message:   "Operation failed",
		Error:     errMsg,
		StartTime: time.Now(),
		EndTime:   time.Now(),
	}

	// Queue result for reporting
	if !a.resultQueue.Enqueue(result) {
		a.logger.Error("Failed to enqueue error result (queue might be closed)",
			"task_id", op.TaskID,
			"error", errMsg)
	}
}

// executeOperation finds and executes the appropriate operation handler
func (a *agent) executeOperation(op *Operation) *OperationResult {
	startTime := time.Now()

	logger := a.logger.With(
		"task_id", op.TaskID,
		"app", op.AppName,
		"resource", op.ResourceID,
		"operation", op.OperationName)

	// Find the app configuration
	appConfig, ok := a.apps[op.AppName]
	if !ok {
		logger.Error("App not found")
		return &OperationResult{
			TaskID:    op.TaskID,
			Status:    "failure",
			Message:   "Operation failed",
			Error:     fmt.Sprintf("app %s not found", op.AppName),
			StartTime: startTime,
			EndTime:   time.Now(),
		}
	}

	// Find the resource definition
	resourceDef, ok := appConfig.GetResourceDefinition(op.ResourceID)
	if !ok {
		logger.Error("Resource not found")
		return &OperationResult{
			TaskID:    op.TaskID,
			Status:    "failure",
			Message:   "Operation failed",
			Error:     fmt.Sprintf("resource %s not found", op.ResourceID),
			StartTime: startTime,
			EndTime:   time.Now(),
		}
	}

	// Find the operation handler
	operation, ok := resourceDef.GetOperation(op.OperationName)
	if !ok {
		logger.Error("Operation not found")
		return &OperationResult{
			TaskID:    op.TaskID,
			Status:    "failure",
			Message:   "Operation failed",
			Error:     fmt.Sprintf("operation %s not found", op.OperationName),
			StartTime: startTime,
			EndTime:   time.Now(),
		}
	}

	ctx := context.Background()

	// Execute the operation
	logger.Info("Executing operation")
	resp, err := operation.Fn(ctx, op.Request)
	endTime := time.Now()

	if err != nil {
		logger.Error("Operation failed", "error", err)
		return &OperationResult{
			TaskID:    op.TaskID,
			Status:    "failure",
			Message:   "Operation failed",
			Error:     err.Error(),
			StartTime: startTime,
			EndTime:   endTime,
		}
	}

	// Create success result
	result := &OperationResult{
		TaskID:    op.TaskID,
		Status:    "success",
		Message:   "Operation completed successfully",
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Extract output if available
	if resp != nil && resp.Data != nil {
		result.Output = extractOutputFromResponseData(resp.Data)
	}

	logger.Info("Operation completed successfully",
		"duration_ms", endTime.Sub(startTime).Milliseconds())

	return result
}

// Helper function to extract output from response data
func extractOutputFromResponseData(data resource.ResponseData) map[string]any {
	output := make(map[string]any)

	if data == nil {
		return output
	}

	switch typedData := data.(type) {
	case *resource.Resource:
		// For single resources, use its properties directly
		if typedData.Properties != nil {
			output = typedData.Properties
		}
	default:
		// Try to handle as a collection
		if collection, ok := data.(*resource.ResourceCollection); ok && collection != nil {
			if len(collection.Items) > 0 {
				// Create collection metadata
				items := make([]map[string]any, 0, len(collection.Items))
				for _, item := range collection.Items {
					if item.Properties != nil {
						items = append(items, item.Properties)
					}
				}

				output = map[string]any{
					"items":      items,
					"count":      len(collection.Items),
					"pagination": collection.Pagination,
				}
			}
		}
	}

	return output
}
