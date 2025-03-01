package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tempestdx/sdk-go/app"
	"github.com/tempestdx/sdk-go/internal/queue"
	"github.com/tempestdx/sdk-go/resource"
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

// agent is the central handler for application operations
type agent struct {
	apiKey        string
	apps          map[string]*appConfig
	opQueue       *queue.TwoLockQueue // Queue for incoming operations
	resultQueue   *queue.TwoLockQueue // Queue for operation results
	server        *http.Server
	mux           *http.ServeMux
	routes        map[string]http.Handler
	workers       int
	resultWorkers int
	workerWg      sync.WaitGroup
	resultWg      sync.WaitGroup
	workerStop    chan struct{}
	resultStop    chan struct{}
	httpClient    *http.Client // Client for API requests
	apiURL        string       // Base URL for Tempest API
	logger        *slog.Logger // Logger for agent operations
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

// Config contains the configuration for an agent
type Config struct {
	APIKey       string
	ServerAddr   string       // Address for the HTTP server, e.g. ":8080"
	QueueOptions QueueOptions // Options for queue processing
	APIURL       string       // Base URL for Tempest API
	Logger       *slog.Logger // Optional custom logger
}

// New creates a new agent with the provided configuration
func New(config Config) *agent {
	mux := http.NewServeMux()

	// Use default queue options if not specified
	queueOpts := config.QueueOptions
	if queueOpts.NumWorkers == 0 {
		queueOpts = DefaultQueueOptions
	}

	apiURL := config.APIURL
	if apiURL == "" {
		apiURL = "https://api.tempestdx.com"
	}

	// Use provided logger or create a default one
	logger := config.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}

	// Create server
	server := &http.Server{
		Addr:    config.ServerAddr,
		Handler: mux,
	}

	return &agent{
		apiKey:        config.APIKey,
		apps:          make(map[string]*appConfig),
		opQueue:       queue.NewTwoLockQueue(),
		resultQueue:   queue.NewTwoLockQueue(),
		server:        server,
		mux:           mux,
		routes:        make(map[string]http.Handler),
		workers:       queueOpts.NumWorkers,
		resultWorkers: queueOpts.NumResultWorkers,
		workerStop:    make(chan struct{}),
		resultStop:    make(chan struct{}),
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		apiURL:        apiURL,
		logger:        logger,
	}
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
	operationRoutes := a.apps[app.Name].generateRoutes()
	canonicalRoutes := a.apps[app.Name].generateCanonicalRoutes()

	a.logger.Debug("Generated routes",
		"app_name", app.Name,
		"operation_routes", len(operationRoutes),
		"canonical_routes", len(canonicalRoutes))

	// Register HTTP handlers for operation routes
	for path := range operationRoutes {
		// Parse elements from path
		// Expected format: appName/version/resourceID/operationName
		parts := strings.Split(path, "/")
		if len(parts) != 4 {
			a.logger.Warn("Invalid path format", "path", path)
			continue // Invalid path format
		}

		appName := parts[0]
		version := parts[1]
		resourceID := parts[2]
		operationName := parts[3]

		// Create and register the operation handler
		a.registerOperationHandler(appName, version, resourceID, operationName)
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

// Start starts the HTTP server and worker goroutines
func (a *agent) Start() error {
	a.logger.Info("Starting agent", "workers", a.workers, "result_workers", a.resultWorkers)

	// Log all registered routes
	routes := make([]string, 0, len(a.routes))
	for route := range a.routes {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	a.logger.Info("Registered routes", "count", len(routes), "routes", routes)

	// Start operation worker goroutines
	for i := 0; i < a.workers; i++ {
		a.workerWg.Add(1)
		go a.operationWorker()
	}

	// Start result reporter goroutines
	for i := 0; i < a.resultWorkers; i++ {
		a.resultWg.Add(1)
		go a.resultReporter()
	}

	// Start the HTTP server
	a.logger.Info("Starting HTTP server", "address", a.server.Addr)
	return a.server.ListenAndServe()
}

// Stop stops the worker goroutines and HTTP server
func (a *agent) Stop(ctx context.Context) error {
	a.logger.Info("Stopping agent")

	// Signal workers to stop
	close(a.workerStop)
	close(a.resultStop)

	a.logger.Debug("Waiting for workers to complete")

	// Wait for workers to finish with a timeout
	workerDone := make(chan struct{})
	resultDone := make(chan struct{})

	go func() {
		a.workerWg.Wait()
		close(workerDone)
	}()

	go func() {
		a.resultWg.Wait()
		close(resultDone)
	}()

	// Wait for both types of workers to finish
	select {
	case <-workerDone:
		a.logger.Debug("Operation workers finished")
	case <-ctx.Done():
		a.logger.Warn("Timeout waiting for operation workers")
	}

	select {
	case <-resultDone:
		a.logger.Debug("Result workers finished")
	case <-ctx.Done():
		a.logger.Warn("Timeout waiting for result workers")
	}

	// Shutdown HTTP server
	a.logger.Info("Shutting down HTTP server")
	return a.server.Shutdown(ctx)
}

// EnqueueOperation adds an operation to the queue for processing
func (a *agent) EnqueueOperation(op *Operation) {
	a.logger.Debug("Enqueueing operation",
		"task_id", op.TaskID,
		"app", op.AppName,
		"operation", op.OperationName,
		"resource_id", op.ResourceID)
	a.opQueue.Enqueue(op)
}

// registerOperationHandler creates and registers a handler for a specific operation
func (a *agent) registerOperationHandler(appName, version, resourceID, operationName string) {
	// Build the route path
	routePath := fmt.Sprintf("/%s/%s/%s/%s", appName, version, resourceID, operationName)

	a.logger.Debug("Registering operation handler",
		"path", routePath,
		"app", appName,
		"version", version,
		"resource", resourceID,
		"operation", operationName)

	// Create the handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Start operation execution time
		startTime := time.Now()

		a.logger.Debug("Handling operation request",
			"path", r.URL.Path,
			"method", r.Method,
			"remote_addr", r.RemoteAddr)

		// Parse request body
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			errMsg := fmt.Sprintf("error reading request: %v", err)
			a.logger.Error("Failed to read request body", "error", err, "path", r.URL.Path)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Parse operation request
		var opReq resource.OperationRequest
		if err := json.Unmarshal(body, &opReq); err != nil {
			errMsg := fmt.Sprintf("error parsing request: %v", err)
			a.logger.Error("Failed to parse request JSON", "error", err, "path", r.URL.Path)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		// Extract task ID from metadata if available
		taskID := ""
		if opReq.Metadata != nil {
			taskID = opReq.Metadata.TaskID
		}

		requestLogger := a.logger.With("task_id", taskID, "operation", operationName)
		requestLogger.Info("Processing operation", "resource_id", resourceID)

		// Find resource definition
		appConfig, ok := a.apps[appName]
		if !ok {
			errMsg := fmt.Sprintf("app %s not found", appName)
			requestLogger.Error("App not found", "app_name", appName)
			http.Error(w, errMsg, http.StatusNotFound)
			return
		}

		resourceDef, ok := appConfig.GetResourceDefinition(resourceID)
		if !ok {
			errMsg := fmt.Sprintf("resource %s not found", resourceID)
			requestLogger.Error("Resource not found", "resource_id", resourceID)
			http.Error(w, errMsg, http.StatusNotFound)
			return
		}

		// Find operation
		operation, ok := resourceDef.GetOperation(operationName)
		if !ok {
			errMsg := fmt.Sprintf("operation %s not found", operationName)
			requestLogger.Error("Operation not found", "operation", operationName)
			http.Error(w, errMsg, http.StatusNotFound)
			return
		}

		// Execute operation
		requestLogger.Debug("Executing operation")
		resp, err := operation.Fn(r.Context(), &opReq)

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
			result.Message = fmt.Sprintf("Operation %s failed: %v", operationName, err)

			requestLogger.Error("Operation failed",
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
		result.Message = fmt.Sprintf("Operation %s completed successfully", operationName)

		// Extract output if available
		if resp != nil && resp.Resource != nil {
			result.Output = resp.Resource.Properties
		}

		requestLogger.Info("Operation completed successfully",
			"duration_ms", duration.Milliseconds())

		// Add result to reporting queue
		a.resultQueue.Enqueue(result)

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			requestLogger.Error("Failed to encode response", "error", err)
			http.Error(w, fmt.Sprintf("error encoding response: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// Register the handler
	a.mux.Handle(routePath, handler)
	a.routes[routePath] = handler
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

	// Sort operations by priority (highest first)
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

// operationWorker pulls operations from the queue and processes them
func (a *agent) operationWorker() {
	defer a.workerWg.Done()

	workerID := fmt.Sprintf("worker-%p", &a.workerWg)
	logger := a.logger.With("worker_id", workerID)

	logger.Debug("Operation worker started")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for {
		select {
		case <-a.workerStop:
			logger.Debug("Operation worker stopping")
			return
		default:
			// Continue processing
		}

		// Dequeue operation
		item, ok := a.opQueue.Dequeue()
		if !ok {
			// Queue is empty, wait before trying again
			time.Sleep(100 * time.Millisecond)
			continue
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
			"app", op.AppName,
			"operation", op.OperationName,
			"resource_id", op.ResourceID)

		opLogger.Debug("Processing queued operation")

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
}

// resultReporter sends operation results to the Tempest API
func (a *agent) resultReporter() {
	defer a.resultWg.Done()

	for {
		select {
		case <-a.resultStop:
			return
		default:
			// Continue processing
		}

		// Dequeue result
		item, ok := a.resultQueue.Dequeue()
		if !ok {
			// Queue is empty, wait before trying again
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Process result
		result, ok := item.(*OperationResult)
		if !ok {
			// Invalid item type, skip
			a.logger.Warn("Invalid item type in result queue", "type", fmt.Sprintf("%T", item))
			continue
		}

		// Send result to Tempest API
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
					routes[fmt.Sprintf("%s/%s/%s/canonical/%s", ac.Name, version, r.UniqueID(), typeStr)] =
						fmt.Sprintf("/%s/%s", ac.Name, r.UniqueID())
				}
			}
		}
	}

	return routes
}

func (ac *appConfig) generateRoutes() map[string]string {
	routes := make(map[string]string)

	for _, r := range ac.ResourceDefinitions() {
		for _, op := range r.Operations() {
			// generate a route per operation, resource, and supported version
			for _, version := range ac.SupportedVersions {
				routes[fmt.Sprintf("%s/%s/%s/%s", ac.Name, version, r.UniqueID(), op.Name())] = fmt.Sprintf("/%s/%s", ac.Name, r.UniqueID())
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

// sortOperationsByPriority sorts operations in descending order of priority
func sortOperationsByPriority(operations []operationMetadata) {
	sort.Slice(operations, func(i, j int) bool {
		// Higher priority comes first
		return operations[i].Priority > operations[j].Priority
	})
}

// Helper function to parse operation requests
func parseOperationRequest(r *http.Request) (*resource.OperationRequest, string, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading request: %v", err)
	}
	defer r.Body.Close()

	var opReq resource.OperationRequest
	if err := json.Unmarshal(body, &opReq); err != nil {
		return nil, "", fmt.Errorf("error parsing request: %v", err)
	}

	// Extract task ID from metadata if available
	taskID := ""
	if opReq.Metadata != nil {
		taskID = opReq.Metadata.TaskID
	}

	return &opReq, taskID, nil
}

// Helper function to create an operation result
func createOperationResult(
	taskID string,
	startTime, endTime time.Time,
	operationName string,
	resp *resource.OperationResponse,
	err error,
	isCanonical bool,
) *OperationResult {
	result := &OperationResult{
		TaskID:    taskID,
		StartTime: startTime,
		EndTime:   endTime,
	}

	if err != nil {
		// Operation failed
		result.Status = "failure"
		result.Error = err.Error()

		if isCanonical {
			result.Message = fmt.Sprintf("All operations for canonical type %s failed", operationName)
		} else {
			result.Message = fmt.Sprintf("Operation %s failed: %v", operationName, err)
		}
	} else {
		// Operation succeeded
		result.Status = "success"

		if isCanonical {
			result.Message = fmt.Sprintf("Operation for canonical type %s completed successfully", operationName)
		} else {
			result.Message = fmt.Sprintf("Operation %s completed successfully", operationName)
		}

		// Extract output if available
		if resp != nil && resp.Resource != nil {
			result.Output = resp.Resource.Properties
		}
	}

	return result
}

// Helper function to send success response to client
func sendSuccessResponse(w http.ResponseWriter, resp *resource.OperationResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, fmt.Sprintf("error encoding response: %v", err), http.StatusInternalServerError)
	}
}
