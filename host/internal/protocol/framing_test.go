package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestReaderRejectsOversizedMessage(t *testing.T) {
	var stream bytes.Buffer
	if err := binary.Write(&stream, binary.LittleEndian, uint32(maxMessageSize+1)); err != nil {
		t.Fatal(err)
	}
	_, err := NewReader(&stream).Read()
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("expected errTooLarge, got %v", err)
	}
}

func TestReaderRejectsInvalidJSON(t *testing.T) {
	payload := "not-json"
	var stream bytes.Buffer
	if err := binary.Write(&stream, binary.LittleEndian, uint32(len(payload))); err != nil {
		t.Fatal(err)
	}
	stream.WriteString(payload)
	_, err := NewReader(&stream).Read()
	if !errors.Is(err, errInvalidJSON) {
		t.Fatalf("expected errInvalidJSON, got %v", err)
	}
}

func TestWriterRejectsLargeMessage(t *testing.T) {
	err := NewWriter(&bytes.Buffer{}).Send(strings.Repeat("x", maxMessageSize+1))
	if !errors.Is(err, errTooLarge) {
		t.Fatalf("expected errTooLarge, got %v", err)
	}
}
