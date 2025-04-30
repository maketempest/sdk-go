package agent

import (
	"log/slog"
	"sync"
)

type WorkerTracker struct {
	wg     sync.WaitGroup
	name   string
	logger *slog.Logger
}

func (w *WorkerTracker) Add(delta int) {
	w.wg.Add(delta)
	w.logger.Debug("Added workers", "name", w.name, "delta", delta)
}

func (w *WorkerTracker) Done() {
	w.wg.Done()
	w.logger.Debug("Worker done", "name", w.name)
}

func (w *WorkerTracker) Wait() {
	w.logger.Debug("Waiting for workers", "name", w.name)
	w.wg.Wait()
	w.logger.Debug("All workers done", "name", w.name)
}
