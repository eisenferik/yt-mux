package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

type Reader struct {
	r io.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (json.RawMessage, error) {
	var size uint32
	if err := binary.Read(r.r, binary.LittleEndian, &size); err != nil {
		return nil, err
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return nil, fmt.Errorf("read native message body: %w", err)
	}
	return json.RawMessage(payload), nil
}

type Writer struct {
	w  io.Writer
	mu sync.Mutex
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) Send(message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode native message: %w", err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := binary.Write(w.w, binary.LittleEndian, uint32(len(payload))); err != nil {
		return fmt.Errorf("write native message length: %w", err)
	}
	if _, err := w.w.Write(payload); err != nil {
		return fmt.Errorf("write native message body: %w", err)
	}
	return nil
}
