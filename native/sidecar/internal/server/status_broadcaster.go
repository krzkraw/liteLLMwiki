package server

import "sync"

type StatusBroadcaster struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func NewStatusBroadcaster() *StatusBroadcaster {
	return &StatusBroadcaster{
		subscribers: make(map[chan struct{}]struct{}),
	}
}

func (b *StatusBroadcaster) Publish() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for subscriber := range b.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}

func (b *StatusBroadcaster) Subscribe() (<-chan struct{}, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan struct{}, 8)
	b.subscribers[ch] = struct{}{}
	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}

	return ch, unsubscribe
}
