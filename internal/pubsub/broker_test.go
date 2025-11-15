package pubsub

import (
	"context"
	"testing"
	"time"
)

func TestBroker_PublishSubscribe(t *testing.T) {
	broker := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe
	ch := broker.Subscribe(ctx)

	// Publish event
	go func() {
		time.Sleep(10 * time.Millisecond)
		broker.Publish(Event[string]{
			Type:    "test.event",
			Payload: "hello",
		})
	}()

	// Receive event
	select {
	case event := <-ch:
		if event.Type != "test.event" {
			t.Errorf("expected type 'test.event', got %s", event.Type)
		}
		if event.Payload != "hello" {
			t.Errorf("expected payload 'hello', got %s", event.Payload)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	broker := NewBroker[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create multiple subscribers
	ch1 := broker.Subscribe(ctx)
	ch2 := broker.Subscribe(ctx)
	ch3 := broker.Subscribe(ctx)

	// Publish event
	broker.Publish(Event[int]{
		Type:    "number",
		Payload: 42,
	})

	// All subscribers should receive
	received := 0
	timeout := time.After(100 * time.Millisecond)

	for received < 3 {
		select {
		case event := <-ch1:
			if event.Payload != 42 {
				t.Errorf("ch1: expected 42, got %d", event.Payload)
			}
			received++
		case event := <-ch2:
			if event.Payload != 42 {
				t.Errorf("ch2: expected 42, got %d", event.Payload)
			}
			received++
		case event := <-ch3:
			if event.Payload != 42 {
				t.Errorf("ch3: expected 42, got %d", event.Payload)
			}
			received++
		case <-timeout:
			t.Fatalf("timeout: only received %d/3 events", received)
		}
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	broker := NewBroker[string]()
	ctx, cancel := context.WithCancel(context.Background())

	ch := broker.Subscribe(ctx)

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after context cancellation")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for channel close")
	}
}

func TestBroker_Close(t *testing.T) {
	broker := NewBroker[string]()
	ctx := context.Background()

	ch1 := broker.Subscribe(ctx)
	ch2 := broker.Subscribe(ctx)

	// Close broker
	broker.Close()

	// All channels should be closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("ch1 should be closed after broker close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for ch1 close")
	}

	select {
	case _, ok := <-ch2:
		if ok {
			t.Error("ch2 should be closed after broker close")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for ch2 close")
	}
}

func TestBroker_SlowSubscriber(t *testing.T) {
	broker := NewBroker[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := broker.Subscribe(ctx)

	// Publish more events than buffer size
	for i := 0; i < 20; i++ {
		broker.Publish(Event[int]{
			Type:    "number",
			Payload: i,
		})
	}

	// Should not block publisher (slow subscribers get skipped)
	// Verify at least some events were received
	timeout := time.After(100 * time.Millisecond)
	received := 0

drainLoop:
	for {
		select {
		case <-ch:
			received++
		case <-timeout:
			break drainLoop
		}
	}

	if received == 0 {
		t.Error("expected to receive at least some events")
	}
	// We expect fewer than 20 due to slow subscriber handling
	t.Logf("received %d/20 events (expected less due to buffer)", received)
}
