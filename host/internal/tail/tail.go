package tail

import (
	"strings"
	"unicode/utf8"
)

const maxLineBytes = 4096

type Buffer struct {
	lines  []string
	limit  int
	redact string
}

func New(limit int, redact string) *Buffer {
	if limit < 1 {
		limit = 1
	}
	return &Buffer{limit: limit, redact: redact}
}

func (b *Buffer) Add(line string) {
	line = sanitize(line)
	if b.redact != "" {
		line = strings.ReplaceAll(line, b.redact, "[temporary cookie file]")
	}
	if line == "" {
		return
	}
	if len(b.lines) == b.limit {
		copy(b.lines, b.lines[1:])
		b.lines[len(b.lines)-1] = line
		return
	}
	b.lines = append(b.lines, line)
}

func (b *Buffer) Lines() []string {
	return append([]string(nil), b.lines...)
}

func sanitize(line string) string {
	line = strings.Map(func(character rune) rune {
		if character == '\t' || character >= 0x20 {
			return character
		}
		return -1
	}, strings.TrimSpace(line))
	if len(line) <= maxLineBytes {
		return line
	}
	line = line[:maxLineBytes]
	for !utf8.ValidString(line) {
		line = line[:len(line)-1]
	}
	return line + "…"
}
