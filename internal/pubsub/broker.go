package pubsub

import (
	"context"
	"sync"
)

// Event represents a generic event with type-safe payload
type Event[T any] struct {
	Type    string
	Payload T
}

// Subscriber interface for consuming events
type Subscriber[T any] interface {
	Subscribe(ctx context.Context) <-chan Event[T]
}

// Broker manages pub/sub for a specific event type
type Broker[T any] struct {
	mu          sync.RWMutex
	subscribers map[chan Event[T]]struct{}
}

// NewBroker creates a new event broker
func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{
		subscribers: make(map[chan Event[T]]struct{}),
	}
}

// Subscribe creates a new subscription channel
func (b *Broker[T]) Subscribe(ctx context.Context) <-chan Event[T] {
	ch := make(chan Event[T], 10)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subscribers, ch)
		close(ch)
		b.mu.Unlock()
	}()

	return ch
}

// Publish sends an event to all subscribers
func (b *Broker[T]) Publish(event Event[T]) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Skip slow subscribers
		}
	}
}

// Close closes all subscriber channels
func (b *Broker[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
}
