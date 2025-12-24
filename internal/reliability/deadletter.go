package reliability

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import (
	"errors"
	"sync"
	"time"
)

var (
	// ErrQueueFull indicates the dead letter queue is full
	ErrQueueFull = errors.New("dead letter queue is full")
)

// FailedOperation represents an operation that failed
type FailedOperation struct {
	// ID is a unique identifier for the operation
	ID string
	// OperationType describes the type of operation
	OperationType string
	// Payload contains the operation data
	Payload interface{}
	// Error is the error that caused the failure
	Error error
	// Timestamp is when the operation failed
	Timestamp time.Time
	// RetryCount is how many times this operation has been retried
	RetryCount int
	// Metadata contains additional context
	Metadata map[string]string
}

// DeadLetterQueue stores failed operations for later analysis or retry
type DeadLetterQueue struct {
	mu         sync.RWMutex
	operations map[string]*FailedOperation
	maxSize    int
	listeners  []func(*FailedOperation)
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(maxSize int) *DeadLetterQueue {
	return &DeadLetterQueue{
		operations: make(map[string]*FailedOperation),
		maxSize:    maxSize,
		listeners:  make([]func(*FailedOperation), 0),
	}
}

// Add adds a failed operation to the queue
func (dlq *DeadLetterQueue) Add(op *FailedOperation) error {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if len(dlq.operations) >= dlq.maxSize {
		return ErrQueueFull
	}

	op.Timestamp = time.Now()
	dlq.operations[op.ID] = op

	// Notify listeners
	for _, listener := range dlq.listeners {
		go listener(op)
	}

	return nil
}

// Get retrieves a failed operation by ID
func (dlq *DeadLetterQueue) Get(id string) (*FailedOperation, bool) {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	op, exists := dlq.operations[id]
	return op, exists
}

// Remove removes a failed operation from the queue
func (dlq *DeadLetterQueue) Remove(id string) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	delete(dlq.operations, id)
}

// List returns all failed operations
func (dlq *DeadLetterQueue) List() []*FailedOperation {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	ops := make([]*FailedOperation, 0, len(dlq.operations))
	for _, op := range dlq.operations {
		ops = append(ops, op)
	}

	return ops
}

// Size returns the current size of the queue
func (dlq *DeadLetterQueue) Size() int {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	return len(dlq.operations)
}

// Clear removes all operations from the queue
func (dlq *DeadLetterQueue) Clear() {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	dlq.operations = make(map[string]*FailedOperation)
}

// AddListener adds a listener that will be called when operations are added
func (dlq *DeadLetterQueue) AddListener(listener func(*FailedOperation)) {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	dlq.listeners = append(dlq.listeners, listener)
}

// GetOldest returns the oldest failed operations up to the specified limit
func (dlq *DeadLetterQueue) GetOldest(limit int) []*FailedOperation {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	ops := make([]*FailedOperation, 0, len(dlq.operations))
	for _, op := range dlq.operations {
		ops = append(ops, op)
	}

	// Sort by timestamp (oldest first)
	for i := 0; i < len(ops)-1; i++ {
		for j := i + 1; j < len(ops); j++ {
			if ops[i].Timestamp.After(ops[j].Timestamp) {
				ops[i], ops[j] = ops[j], ops[i]
			}
		}
	}

	if len(ops) > limit {
		ops = ops[:limit]
	}

	return ops
}

// GetByType returns all failed operations of a specific type
func (dlq *DeadLetterQueue) GetByType(operationType string) []*FailedOperation {
	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	ops := make([]*FailedOperation, 0)
	for _, op := range dlq.operations {
		if op.OperationType == operationType {
			ops = append(ops, op)
		}
	}

	return ops
}
