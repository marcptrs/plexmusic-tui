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
	closed      bool
	closedCh    chan Event[T]
}

// NewBroker creates a new event broker
func NewBroker[T any]() *Broker[T] {
	// Pre-create a closed channel that can be returned to callers after the
	// broker has been closed. This avoids creating ad-hoc closed channels on
	// every Subscribe after Close and provides a single closed sentinel that
	// is safely reused.
	closedCh := make(chan Event[T])
	close(closedCh)

	return &Broker[T]{
		subscribers: make(map[chan Event[T]]struct{}),
		closedCh:    closedCh,
	}
}

// Subscribe creates a new subscription channel
func (b *Broker[T]) Subscribe(ctx context.Context) <-chan Event[T] {
	ch := make(chan Event[T], 10)

	b.mu.Lock()
	// If the broker is already closed, return a closed channel immediately
	// instead of registering a new subscriber.
	if b.closed {
		closed := b.closedCh
		b.mu.Unlock()
		return closed
	}
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		b.unsubscribe(ch)
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

/*
unsubscribe safely removes a subscription and closes the channel unless
the broker is already in the process of shutting down.
*/
func (b *Broker[T]) unsubscribe(ch chan Event[T]) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// If the channel is not in the map, nothing to do.
	if _, ok := b.subscribers[ch]; !ok {
		return
	}

	// Remove the subscription entry.
	delete(b.subscribers, ch)

	// Only close the channel if the broker isn't already closing.
	if !b.closed {
		close(ch)
	}
}

/*
Close closes all subscriber channels and marks the broker as closed so
concurrent unsubscribe() calls don't attempt to close channels that were
already closed by this function.
*/
func (b *Broker[T]) Close() {
	b.mu.Lock()
	// If already closed, nothing to do.
	if b.closed {
		b.mu.Unlock()
		return
	}
	// Mark broker as closed while we perform the close operations so that
	// subscribe/unsubscribe goroutines will avoid double-closing channels.
	b.closed = true

	for ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, ch)
	}
	b.mu.Unlock()
}
