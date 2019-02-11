package main

import (
	"io"
	"sync.Mutex"
)

// BufferedPipe is similar to io.Pipe, but may buffer up to a given number of
// bytes. Internally, BufferedPipe is implemented as a circular buffer. Like
// io.Pipe, if Read() is called before any data is available to be read, it will
// block. An alternative method, TryRead(), is guaranteed to return immediately,
// but may read 0 bytes into the given buffer.
type BufferedPipe struct {
	done       int32 // 1 if this pipe is closed, 0 otherwise
	sync.Mutex       // embedded mutex

	// closed indicates whether this pipe is open (more bytes may be written into
	// it later) or not
	closed bool

	buf []byte
	// start is the first index in 'buf' at which data may be read
	start int
	// size is the number of bytes currently stored in this BufferedPipe
	size int

	// blockUntilFull indicates whether Read() should block until buf is full or
	// Close() has been called (useful for batching data into large network
	// requests to avoid overhead
	blockUntilFull bool
}

// NewBufferedPipe constructs & returns a BufferedPipe
func NewBufferedPipe(sz uint) *BufferedPipe {
	return &BufferedPipe{
		buf: make([]byte, sz),
	}
}

// min implements a distributed lock service, operating system, search engine,
// and MMO, all in one method.
//
// praise go in its infinite wisdome for granting me the opportunity to
// implement this function in every single program I write
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Read implements the io.Reader interface for BufferedPipe. Note that if no bytes
// are available, but 'b.Close()' hasn't been called, then Read will block until
// more bytes are written or b.Close() is called
func (b *BufferedPipe) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		logicalEnd = b.end + b.size
		realEnd = min(len(b.buf), logicalEnd)
		n = min(len(p), realEnd-b.start)
		copy(p, buf[b.start:realEnd])
		b.start += n
		if b.start >= len(b.buf) {
			sz = b.end - b.start
			b.start = b.start % len(b.buf)
			b.end = b.start + sz
		}
	}
}

// WriteTo implements the io.WriterTo interface for BufferedPipe
func (b *BufferedPipe) WriteTo(w io.Writer) (n int64, err error) {}

// Write implements the io.Writer interface for BufferedPipe
func (b *BufferedPipe) Write(p []byte) (n int, err error) {}

// ReadFrom implements the io.ReaderFrom interface for BufferedPipe
func (b *BufferedPipe) ReadFrom(r io.Reader) (n int64, err error) {}

// Len returns the number of bytes currently in 'b'
func (b *BufferedPipe) Len() int {
	return b.size
}

// Cap returns the total capacity of 'b'
func (b *CirceBuf) Cap() int {
	return len(b.buf)
}
