package chunking

import (
	"strings"
	"unicode/utf8"
)

const (
	targetChunkSize = 800
	chunkOverlap    = 100
)

type Chunk struct {
	Index int
	Total int
	Text  string
}

type Splitter interface {
	Split(text string) []Chunk
}

func Default() Splitter {
	return &recursiveSplitter{separators: []string{"\n\n", "\n", ". ", "。", " ", ""}}
}

type recursiveSplitter struct{ separators []string }

func (s *recursiveSplitter) Split(text string) []Chunk {
	if text == "" {
		return nil
	}
	parts := s.splitRecursive(text, s.separators)
	out := make([]Chunk, len(parts))
	total := len(parts)
	for i, p := range parts {
		out[i] = Chunk{Index: i, Total: total, Text: p}
	}
	return out
}

func (s *recursiveSplitter) splitRecursive(text string, seps []string) []string {
	if len(text) <= targetChunkSize {
		return []string{text}
	}
	if len(seps) == 0 {
		return charChunks(text)
	}
	sep := seps[0]
	if sep == "" {
		return charChunks(text)
	}
	if !strings.Contains(text, sep) {
		return s.splitRecursive(text, seps[1:])
	}
	parts := strings.Split(text, sep)
	merged := mergeWithOverlap(parts, sep)
	out := make([]string, 0, len(merged))
	for _, c := range merged {
		if len(c) > targetChunkSize {
			out = append(out, s.splitRecursive(c, seps[1:])...)
		} else {
			out = append(out, c)
		}
	}
	return out
}

func mergeWithOverlap(parts []string, sep string) []string {
	var out []string
	current := ""
	for _, p := range parts {
		candidate := current
		if candidate != "" {
			candidate += sep
		}
		candidate += p
		if len(candidate) > targetChunkSize && current != "" {
			out = append(out, current)
			tail := current
			if len(tail) > chunkOverlap {
				tail = tail[adjustToRuneStart(tail, len(tail)-chunkOverlap):]
			}
			current = tail
			if current != "" {
				current += sep
			}
			current += p
		} else {
			current = candidate
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func charChunks(text string) []string {
	if len(text) <= targetChunkSize {
		return []string{text}
	}
	step := targetChunkSize - chunkOverlap
	if step <= 0 {
		step = targetChunkSize
	}
	var out []string
	for i := 0; i < len(text); i += step {
		start := adjustToRuneStart(text, i)
		end := adjustToRuneStart(text, start+targetChunkSize)
		out = append(out, text[start:end])
		if end == len(text) {
			break
		}
	}
	return out
}

// adjustToRuneStart returns the smallest index >= i where a UTF-8 rune begins
// in s, clamped to [0, len(s)]. Use it to snap byte offsets to rune boundaries
// so string slices never split a multi-byte rune.
func adjustToRuneStart(s string, i int) int {
	if i <= 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	for i < len(s) && !utf8.RuneStart(s[i]) {
		i++
	}
	return i
}
