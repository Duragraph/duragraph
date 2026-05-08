package watch

import (
	"bytes"
	"io"
	"sync"
)

// prefixWriter wraps an io.Writer and prepends a fixed prefix to every
// complete line it forwards. Subprocess stdout/stderr arrives in
// arbitrary chunks (could be a partial line, several lines, or a line
// without a trailing newline), so the writer line-buffers: complete
// lines (terminated by '\n') are flushed with the prefix, and any
// trailing bytes without a newline are held until the next Write or
// an explicit Flush.
//
// Concurrent writes are serialised with a mutex; cmd.Stdout and
// cmd.Stderr both pointing at the same writer is the common case and
// must not interleave mid-line.
type prefixWriter struct {
	mu     sync.Mutex
	prefix []byte
	w      io.Writer
	buf    bytes.Buffer
}

// newPrefixWriter returns a prefixWriter that emits to w with the given
// prefix. The prefix is captured by value, so callers may reuse the
// argument string after construction.
func newPrefixWriter(prefix string, w io.Writer) *prefixWriter {
	return &prefixWriter{prefix: []byte(prefix), w: w}
}

// Write implements io.Writer. It emits one prefixed line per '\n' in
// the input and buffers any tail. The returned n always equals len(p)
// on success — to upstream consumers (cmd.Stdout) it looks like a
// passthrough writer.
func (p *prefixWriter) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(b)
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			// No newline left — stash and report full consumption.
			p.buf.Write(b)
			return n, nil
		}
		// Emit prefix + buffered tail + this line (incl. \n).
		if _, err := p.w.Write(p.prefix); err != nil {
			return 0, err
		}
		if p.buf.Len() > 0 {
			if _, err := p.w.Write(p.buf.Bytes()); err != nil {
				return 0, err
			}
			p.buf.Reset()
		}
		if _, err := p.w.Write(b[:i+1]); err != nil {
			return 0, err
		}
		b = b[i+1:]
	}
}

// Flush emits any buffered tail (a partial line without a trailing
// newline) with the prefix, appending a synthetic '\n' so the output
// stream remains well-formed. Called when the supervised process exits
// — without this, a worker that crashes mid-line would leave its last
// fragment hidden.
func (p *prefixWriter) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.buf.Len() == 0 {
		return nil
	}
	if _, err := p.w.Write(p.prefix); err != nil {
		return err
	}
	if _, err := p.w.Write(p.buf.Bytes()); err != nil {
		return err
	}
	if _, err := p.w.Write([]byte{'\n'}); err != nil {
		return err
	}
	p.buf.Reset()
	return nil
}
