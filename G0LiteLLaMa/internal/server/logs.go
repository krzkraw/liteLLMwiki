package server

import (
	"bytes"
	"io"
	"strings"
	"sync"

	"g0litellama/internal/redact"
)

type LogEntry struct {
	Seq    uint64 `json:"seq"`
	Source string `json:"source"`
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

type LogBroadcaster struct {
	mu          sync.Mutex
	capacity    int
	nextSeq     uint64
	ring        []LogEntry
	subscribers map[chan LogEntry]struct{}
}

func NewLogBroadcaster(capacity int) *LogBroadcaster {
	if capacity <= 0 {
		capacity = 1
	}

	return &LogBroadcaster{
		capacity:    capacity,
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

func (b *LogBroadcaster) Publish(source string, stream string, line string) LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextSeq++
	entry := LogEntry{
		Seq:    b.nextSeq,
		Source: source,
		Stream: stream,
		Line:   redact.FromEnv(strings.TrimSuffix(line, "\r")),
	}
	if len(b.ring) == b.capacity {
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = entry
	} else {
		b.ring = append(b.ring, entry)
	}
	for subscriber := range b.subscribers {
		select {
		case subscriber <- entry:
		default:
		}
	}

	return entry
}

func (b *LogBroadcaster) Subscribe() ([]LogEntry, <-chan LogEntry, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	snapshot := append([]LogEntry(nil), b.ring...)
	ch := make(chan LogEntry, 32)
	b.subscribers[ch] = struct{}{}
	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()

		if _, ok := b.subscribers[ch]; ok {
			delete(b.subscribers, ch)
			close(ch)
		}
	}

	return snapshot, ch, unsubscribe
}

func (b *LogBroadcaster) Snapshot() []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()

	return append([]LogEntry(nil), b.ring...)
}

func (b *LogBroadcaster) LatestSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.nextSeq
}

func (b *LogBroadcaster) Writer(source string, stream string, tee io.Writer) io.Writer {
	return &logWriter{
		broadcaster: b,
		source:      source,
		stream:      stream,
		tee:         tee,
	}
}

type logWriter struct {
	mu          sync.Mutex
	broadcaster *LogBroadcaster
	source      string
	stream      string
	tee         io.Writer
	buffer      bytes.Buffer
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n := len(p)
	return n, w.append(p)
}

func (w *logWriter) append(p []byte) error {
	var firstErr error
	for len(p) > 0 {
		index := bytes.IndexByte(p, '\n')
		if index < 0 {
			_, _ = w.buffer.Write(p)
			return firstErr
		}

		_, _ = w.buffer.Write(p[:index])
		entry := w.broadcaster.Publish(w.source, w.stream, w.buffer.String())
		if w.tee != nil {
			if _, err := io.WriteString(w.tee, entry.Line+"\n"); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		w.buffer.Reset()
		p = p[index+1:]
	}

	return firstErr
}
