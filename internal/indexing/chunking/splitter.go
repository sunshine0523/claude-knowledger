package chunking

import "strings"

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
				tail = tail[len(tail)-chunkOverlap:]
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
		end := i + targetChunkSize
		if end > len(text) {
			end = len(text)
		}
		out = append(out, text[i:end])
		if end == len(text) {
			break
		}
	}
	return out
}
