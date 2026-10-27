package syncbuffer

import (
	"bytes"
	"sync"
)

// Buffer preserves the terminal-style combined output while
// allowing stdout and stderr copy loops to write concurrently.
type Buffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	Limit  int // Set before the first Write. Zero means unlimited; do not change during use.
}

func (buffer *Buffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	n := len(value)
	if buffer.Limit > 0 {
		remaining := max(0, buffer.Limit-buffer.buffer.Len())
		value = value[:min(len(value), remaining)]
	}
	_, err := buffer.buffer.Write(value)
	return n, err
}

func (buffer *Buffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
