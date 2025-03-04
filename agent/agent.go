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

type appConfig struct {
	*app.App
	SupportedVersions []string
	CanonicalBindings map[resourceName]map[resource.CanonicalType][]canonicalBinding
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
	TaskID    string                 `json:"task_id"`
	Status    string                 `json:"status"`           // "success", "failure"
	Message   string                 `json:"message"`          // Human-readable message
	Error     string                 `json:"error,omitempty"`  // Error details if failed
	Output    map[string]interface{} `json:"output,omitempty"` // Operation output data
	StartTime time.Time              `json:"start_time"`
	EndTime   time.Time              `json:"end_time"`
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
	workerWg       sync.WaitGroup
	resultWg       sync.WaitGroup
	workerStop     chan struct{}
	resultStop     chan struct{}
	httpClient     *http.Client // Client for API requests
	apiURL         string       // Base URL for Tempest API
	logger         *slog.Logger // Logger for agent operations
	pollTimeout    time.Duration
	stopOptions    StopOptions // Default options for stopping the agent
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
func New(options ...Option) *agent {
	// Read configuration from environment variables with defaults
	apiKey := getEnv("TEMPEST_API_KEY", "")
	serverAddr := getEnv("TEMPEST_SERVER_ADDR", ":8080")
	apiURL := getEnv("TEMPEST_API_URL", "https://api.tempestdx.com")
	numWorkers := getEnvInt("TEMPEST_NUM_WORKERS", DefaultQueueOptions.NumWorkers)
	numResultWorkers := getEnvInt("TEMPEST_NUM_RESULT_WORKERS", DefaultQueueOptions.NumResultWorkers)
	pollTimeout := getEnvDuration("TEMPEST_POLL_TIMEOUT", DefaultQueueOptions.PollTimeout)
	logLevel := getEnvLogLevel("TEMPEST_LOG_LEVEL", slog.LevelInfo)

	// Determine log format from environment
	logFormat := LogFormatJSON // Default to JSON logging
	if format := strings.ToLower(getEnv("TEMPEST_LOG_FORMAT", "json")); format == "text" {
		logFormat = LogFormatText
	}

	// Create HTTP server and mux
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	// Create logger with environment settings
	var logger *slog.Logger
	var handler slog.Handler

	if logFormat == LogFormatJSON {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		})
	} else {
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: logLevel,
		})
	}
	logger = slog.New(handler)

	// Initialize the agent with environment-based defaults
	a := &agent{
		apiKey:         apiKey,
		apps:           make(map[string]*appConfig),
		resourceQueues: queue.NewResourceQueueManager(),
		resultQueue:    queue.NewQueue(1000),
		pollers:        make(map[string]context.CancelFunc),
		server:         server,
		mux:            mux,
		routes:         make(map[string]http.Handler),
		workers:        numWorkers,
		resultWorkers:  numResultWorkers,
		pollTimeout:    pollTimeout,
		workerStop:     make(chan struct{}),
		resultStop:     make(chan struct{}),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiURL:      apiURL,
		logger:      logger,
		stopOptions: DefaultStopOptions,
	}

	// Apply provided options after environment defaults
	for _, option := range options {
		option(a)
	}

	// Apply default routes to the mux
	a.registerDefaultHandlers()

	return a
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
	operationsPrefixRoutes := a.apps[app.Name].generateOperationRoutes()

	a.logger.Debug("Generated routes",
		"app_name", app.Name,
		"operations_prefix_routes", len(operationsPrefixRoutes),
		"canonical_routes", len(canonicalRoutes))

	// Register HTTP handlers for operation routes with /operations/ prefix
	for path := range operationsPrefixRoutes {
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
		a.registerOperationPrefixHandler(appName, version, resourceID, operationName)
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
		a.registerCanonicalHandler(appName, version, resourceID, canonicalTypeStr)
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
//	agent := tempest.NewAgent()
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
	a.resultWg.Add(a.resultWorkers)
	for i := 0; i < a.resultWorkers; i++ {
		go a.resultReporter()
	}

	// This is where we would start the task poller to continuously poll the Tempest API
	// for new operations to process. The taskPoller would fetch tasks from the API and
	// route them to the appropriate resource queues.
	// go a.taskPoller()

	// Start HTTP server in a goroutine
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("Error starting HTTP server", "error", err)
		}
	}()

	a.logger.Info("Agent started", "api_url", a.apiURL)

	return nil
}

// Stop gracefully shuts down the agent with default options
func (a *agent) Stop(ctx context.Context) error {
	return a.StopWithOptions(ctx, a.stopOptions)
}

// StopWithOptions gracefully shuts down the agent with custom options
func (a *agent) StopWithOptions(ctx context.Context, options StopOptions) error {
	a.logger.Info("Stopping agent", "no_timeout", options.NoTimeout)

	// First shut down the HTTP server with a timeout
	var serverTimeout time.Duration
	if options.NoTimeout {
		serverTimeout = 30 * time.Second // Still use a reasonable timeout for HTTP server
	} else {
		serverTimeout = options.Timeout
	}

	serverCtx, serverCancel := context.WithTimeout(ctx, serverTimeout)
	defer serverCancel()

	if err := a.server.Shutdown(serverCtx); err != nil {
		a.logger.Error("Error shutting down HTTP server", "error", err)
	}

	// Use our shutdown function which sends all the stop signals
	a.shutdown()

	// If NoTimeout is true, wait indefinitely for workers to finish
	if options.NoTimeout {
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
		a.workerWg.Wait()
		a.resultWg.Wait()

		// Stop the watchdog
		close(done)
		a.logger.Info("All workers finished gracefully")
	} else {
		// Use a timeout for waiting
		workerDone := make(chan struct{})
		go func() {
			a.workerWg.Wait()
			a.resultWg.Wait()
			close(workerDone)
		}()

		select {
		case <-workerDone:
			a.logger.Info("All workers finished within timeout")
		case <-time.After(options.Timeout):
			a.logger.Warn("Timed out waiting for workers to finish, forcing exit",
				"timeout_seconds", options.Timeout.Seconds())
		}
	}

	a.logger.Info("Agent stopped")
	return nil
}

// registerOperationHandler creates and registers a handler for a specific operation
func (a *agent) registerOperationHandler(appName, version, resourceID, operationName string) {
	path := fmt.Sprintf("/%s/%s/%s/%s", appName, version, resourceID, operationName)
	a.logger.Debug("Registering operation handler", "path", path)

	handler := &operationHandler{
		agent:         a,
		appName:       appName,
		version:       version,
		resourceID:    resourceID,
		operationName: operationName,
	}

	a.mux.HandleFunc(path, handler.ServeHTTP)
	a.routes[path] = handler
}

// registerOperationPrefixHandler creates and registers a handler for an operation with the /operations/ prefix
func (a *agent) registerOperationPrefixHandler(appName, version, resourceID, operationName string) {
	path := fmt.Sprintf("/%s/%s/%s/operations/%s", appName, version, resourceID, operationName)
	a.logger.Debug("Registering operation prefix handler", "path", path)

	handler := &operationHandler{
		agent:         a,
		appName:       appName,
		version:       version,
		resourceID:    resourceID,
		operationName: operationName,
	}

	a.mux.HandleFunc(path, handler.ServeHTTP)
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
	if resp != nil && resp.Resource != nil {
		result.Output = resp.Resource.Properties
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

// registerCanonicalHandler creates and registers a handler for a canonical operation type
func (a *agent) registerCanonicalHandler(appName, version, resourceID, canonicalTypeStr string) {
	// Build the route path
	routePath := fmt.Sprintf("/%s/%s/%s/canonical/%s", appName, version, resourceID, canonicalTypeStr)

	a.logger.Debug("Registering canonical handler",
		"path", routePath,
		"app", appName,
		"version", version,
		"resource", resourceID,
		"canonical_type", canonicalTypeStr)

	// Create the handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			"canonical_type", canonicalTypeStr,
			"resource_id", resourceID)

		requestLogger.Info("Processing canonical operation")

		// Execute canonical operation
		resp, err := a.executeCanonicalOperation(
			r.Context(),
			appName,
			resourceID,
			canonicalTypeStr,
			opReq,
			taskID,
			startTime,
		)

		// End operation execution time
		endTime := time.Now()
		duration := endTime.Sub(startTime)

		// Only create overall result if we need to report failure
		// (Success is already reported by executeCanonicalOperation)
		if err != nil {
			// Create failure result object for reporting
			result := &OperationResult{
				TaskID:    taskID,
				Status:    "failure",
				Message:   fmt.Sprintf("Canonical operation %s failed: %v", canonicalTypeStr, err),
				Error:     err.Error(),
				StartTime: startTime,
				EndTime:   endTime,
			}
			a.resultQueue.Enqueue(result)

			requestLogger.Error("Canonical operation failed",
				"error", err.Error(),
				"duration_ms", duration.Milliseconds())

			// Handle response to client
			http.Error(w, fmt.Sprintf("operation failed: %v", err), http.StatusInternalServerError)
			return
		}

		requestLogger.Info("Canonical operation completed successfully",
			"duration_ms", duration.Milliseconds())

		sendSuccessResponse(w, resp)
	})

	// Register the handler
	a.mux.Handle(routePath, handler)
	a.routes[routePath] = handler
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
	bindings, ok := appConfig.CanonicalBindings[resName][canonicalType]
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
		if resp != nil && resp.Resource != nil {
			opResult.Output = resp.Resource.Properties
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
	if lastResponse != nil && lastResponse.Resource != nil {
		finalResult.Output = lastResponse.Resource.Properties
	}

	a.resultQueue.Enqueue(finalResult)
	logger.Info("All operations in canonical sequence completed successfully")

	return lastResponse, nil
}

// resultReporter sends operation results to the Tempest API
func (a *agent) resultReporter() {
	defer a.resultWg.Done()

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
		var item interface{}
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
	ac.CanonicalBindings = make(map[resourceName]map[resource.CanonicalType][]canonicalBinding)

	for _, r := range ac.ResourceDefinitions() {
		resName := resourceName(r.UniqueID())
		ac.CanonicalBindings[resName] = make(map[resource.CanonicalType][]canonicalBinding)

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
			ac.CanonicalBindings[resName][canonicalType] = []canonicalBinding{
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
	var rawData map[string]interface{}
	if err := json.Unmarshal(body, &rawData); err != nil {
		return nil, "", fmt.Errorf("error parsing request: %v", err)
	}

	// Create operation request
	opReq := &resource.OperationRequest{
		Metadata: &resource.Metadata{},
	}

	// Check if this is the standard API format with nested "task" structure
	if taskRaw, hasTask := rawData["task"]; hasTask {
		task, ok := taskRaw.(map[string]interface{})
		if !ok {
			return nil, "", fmt.Errorf("task field is not a valid object")
		}

		// Map task.input to Args
		if input, hasInput := task["input"].(map[string]interface{}); hasInput {
			opReq.Args = input
		}

		// Map task.resource to Resource
		if resourceRaw, hasResource := task["resource"].(map[string]interface{}); hasResource {
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
			if props, ok := resourceRaw["properties"].(map[string]interface{}); ok {
				res.Properties = props
			}

			// Add resource to request
			opReq.Resource = res
		}

		// Map task.environment_variables (if supported by OperationRequest)
		if envVarsRaw, hasEnvVars := task["environment_variables"].([]interface{}); hasEnvVars {
			// Check if your OperationRequest has a field for environment variables
			// If so, map them here
			envVars := make(map[string]resource.EnvironmentVariable)
			for _, envVar := range envVarsRaw {
				if envVarMap, ok := envVar.(map[string]interface{}); ok {
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
		if metadataRaw, hasMetadata := task["metadata"].(map[string]interface{}); hasMetadata {
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
	if metadataRaw, hasMetadata := rawData["metadata"].(map[string]interface{}); hasMetadata {
		opReq.Metadata.Owners = []resource.Owner{}
		// Extract author info
		// TODO: Expand owner support to include all owners supported on server
		if authorRaw, hasAuthor := metadataRaw["author"].(map[string]interface{}); hasAuthor {
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

// startResourcePoller creates and starts a dedicated poller for a specific app/resource combination.
// Each resource gets its own dedicated poller that processes operations sequentially, ensuring
// that operations on the same resource are executed in order. Multiple resource pollers run
// concurrently, allowing operations on different resources to be processed in parallel.
//
// The pollers are started automatically by the agent when operations are enqueued, and
// the agent manages their lifecycle. SDK users do not need to start or manage these pollers manually.
func (a *agent) startResourcePoller(appName, resourceID string) {
	key := appName + "/" + resourceID

	// Check if a poller is already running for this resource
	a.pollersMutex.RLock()
	_, exists := a.pollers[key]
	a.pollersMutex.RUnlock()

	if exists {
		return // Already running
	}

	// Create a new context for this poller
	ctx, cancel := context.WithCancel(context.Background())

	a.pollersMutex.Lock()
	// Double-check in case another goroutine started it while we were getting the lock
	if _, exists := a.pollers[key]; exists {
		cancel() // Don't need this one
		a.pollersMutex.Unlock()
		return
	}

	// Register the cancel function
	a.pollers[key] = cancel
	a.pollersMutex.Unlock()

	// Get queue again to be safe
	resourceQueue := a.resourceQueues.GetOrCreateQueue(appName, resourceID, 1000)

	// Increment worker wait group for this poller
	a.workerWg.Add(1)

	// Start the processor goroutine for this resource queue
	go func() {
		// Ensure we decrement the wait group when done
		defer a.workerWg.Done()

		logger := a.logger.With(
			"poller", key,
			"app", appName,
			"resource_id", resourceID)

		logger.Debug("Starting resource poller")

		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		for {
			select {
			case <-ctx.Done():
				logger.Debug("Resource poller stopping")
				return
			default:
				// Continue processing
			}

			// Dequeue operation with timeout to allow checking context cancellation
			var item interface{}
			var ok bool
			select {
			case <-ctx.Done():
				logger.Debug("Resource poller stopping during dequeue")
				return
			case <-time.After(100 * time.Millisecond):
				// Try to dequeue without blocking
				item, ok = resourceQueue.TryDequeue()
				if !ok {
					// Queue is empty, wait before trying again
					select {
					case <-time.After(100 * time.Millisecond):
					case <-ctx.Done():
						return
					}
					continue
				}
			}

			// Process operation
			op, ok := item.(*Operation)
			if !ok {
				// Invalid item type, skip
				logger.Warn("Invalid item type in operation queue", "type", fmt.Sprintf("%T", item))
				continue
			}

			opLogger := logger.With(
				"task_id", op.TaskID,
				"operation", op.OperationName)

			opLogger.Debug("Processing queued operation")

			// Process the operation using the same logic as in operationWorker
			// Construct path for the operation
			path := fmt.Sprintf("/%s/%s/%s/%s",
				op.AppName, op.Version, op.ResourceID, op.OperationName)

			// Prepare HTTP request to local server
			data, err := json.Marshal(op.Request)
			if err != nil {
				opLogger.Error("Failed to marshal operation request", "error", err)
				continue
			}

			httpReq, err := http.NewRequest("POST", "http://localhost"+a.server.Addr+path, bytes.NewBuffer(data))
			if err != nil {
				opLogger.Error("Failed to create HTTP request", "error", err, "path", path)
				continue
			}

			httpReq.Header.Set("Content-Type", "application/json")

			// Execute request
			startTime := time.Now()
			resp, err := client.Do(httpReq)
			duration := time.Since(startTime)

			if err != nil {
				opLogger.Error("Failed to execute operation request",
					"error", err,
					"path", path,
					"duration_ms", duration.Milliseconds())
				continue
			}

			statusOK := resp.StatusCode >= 200 && resp.StatusCode < 300
			if !statusOK {
				body, _ := io.ReadAll(resp.Body)
				opLogger.Error("Operation request failed",
					"status_code", resp.StatusCode,
					"response", string(body),
					"duration_ms", duration.Milliseconds())
			} else {
				opLogger.Info("Operation request completed successfully",
					"status_code", resp.StatusCode,
					"duration_ms", duration.Milliseconds())
			}

			resp.Body.Close()
		}
	}()
}

// shutdown properly closes all resource queues and stops all pollers.
// This is called internally during agent shutdown to ensure proper cleanup
// of all resource pollers and queues. It cancels all poller contexts,
// closes all queues, and ensures that resources are properly released.
// SDK users do not need to call this method directly, as it's handled
// automatically during graceful shutdown.
func (a *agent) shutdown() {
	a.logger.Info("Shutting down agent")

	// First stop all pollers (which consume from resource queues)
	// This ensures they won't pull any new operations
	a.pollersMutex.Lock()
	for _, cancel := range a.pollers {
		cancel()
	}
	// Clear the pollers map to prevent double cancellation
	a.pollers = make(map[string]context.CancelFunc)
	a.pollersMutex.Unlock()

	// Signal workers to stop - with nil check
	if a.workerStop != nil {
		select {
		case <-a.workerStop: // Already closed
		default:
			close(a.workerStop)
		}
	}

	// Close all resource queues - this will cause any remaining Dequeue operations to return
	if a.resourceQueues != nil {
		a.logger.Debug("Closing resource queues")
		a.resourceQueues.Close()
	}

	// Wait a moment for in-flight operations to finish enqueuing results
	time.Sleep(100 * time.Millisecond)

	// Signal result reporters to stop - with nil check
	if a.resultStop != nil {
		select {
		case <-a.resultStop: // Already closed
		default:
			close(a.resultStop)
		}
	}

	// Close the result queue last after all workers have stopped submitting results
	if a.resultQueue != nil {
		a.logger.Debug("Closing result queue")
		a.resultQueue.Close()
	}

	a.logger.Debug("All queues closed")
}

// taskPoller continuously polls the Tempest API for new tasks and enqueues
// them to the appropriate resource queues. It handles fetching pending operations
// from the API, determining the target app/resource, and routing each operation
// to the correct queue.
//
// This function runs as a goroutine and is started automatically when the agent
// starts. It ensures operations are processed in the correct order per resource
// while allowing concurrent processing across different resources.
//
// SDK users do not need to implement or call this method, as it's handled
// automatically by the agent.
// func (a *agent) taskPoller() {
// Implementation will poll the Tempest API for tasks
// and enqueue them to the appropriate resource queues

// For now, this is a stub - the actual implementation will be added later
// }

// registerDefaultHandlers registers the default HTTP handlers for the agent
func (a *agent) registerDefaultHandlers() {
	// Register the describe route
	a.registerDescribeHandler()
}
