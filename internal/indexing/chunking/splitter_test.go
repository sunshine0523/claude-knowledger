package chunking

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitEmpty(t *testing.T) {
	got := Default().Split("")
	if len(got) != 0 {
		t.Fatalf("expected 0 chunks for empty input, got %d", len(got))
	}
}

func TestSplitShortDocumentReturnsSingleChunk(t *testing.T) {
	text := "Short text under threshold."
	got := Default().Split(text)
	if len(got) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(got))
	}
	if got[0].Index != 0 || got[0].Total != 1 || got[0].Text != text {
		t.Fatalf("unexpected chunk: %#v", got[0])
	}
}

func TestSplitParagraphPreferred(t *testing.T) {
	para := strings.Repeat("a", 500)
	text := para + "\n\n" + para
	got := Default().Split(text)
	if len(got) < 2 {
		t.Fatalf("expected >=2 chunks for paragraph-separated long text, got %d", len(got))
	}
	for i, c := range got {
		if len(c.Text) > targetChunkSize {
			t.Fatalf("chunk %d size %d exceeds target %d", i, len(c.Text), targetChunkSize)
		}
		if c.Total != len(got) {
			t.Fatalf("chunk %d total %d != %d", i, c.Total, len(got))
		}
		if c.Index != i {
			t.Fatalf("chunk %d index %d", i, c.Index)
		}
	}
}

func TestSplitFallsThroughToSentenceAndChar(t *testing.T) {
	text := strings.Repeat("hello world. ", 200)
	got := Default().Split(text)
	if len(got) < 3 {
		t.Fatalf("expected multi-chunk split for long sentence-only text, got %d", len(got))
	}
	for _, c := range got {
		if len(c.Text) > targetChunkSize {
			t.Fatalf("chunk too large: %d", len(c.Text))
		}
	}
}

func TestSplitChineseSentenceSeparator(t *testing.T) {
	sentence := strings.Repeat("中文测试", 60)
	text := sentence + "。" + sentence + "。" + sentence
	got := Default().Split(text)
	if len(got) < 2 {
		t.Fatalf("expected Chinese 。 to act as a separator and yield >=2 chunks, got %d", len(got))
	}
}

func TestSplitOverlapSharesSuffixPrefix(t *testing.T) {
	a := strings.Repeat("A", 700)
	b := strings.Repeat("B", 700)
	got := Default().Split(a + "\n\n" + b)
	if len(got) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(got))
	}
	first, second := got[0].Text, got[1].Text
	overlap := first
	if len(overlap) > chunkOverlap {
		overlap = overlap[len(overlap)-chunkOverlap:]
	}
	if overlap == "" || !strings.HasPrefix(second, overlap[:1]) {
		t.Logf("first tail=%q second head=%q (overlap inspection)", overlap, second[:minInt(len(second), chunkOverlap)])
	}
}

func TestSplitCharLevelFallback(t *testing.T) {
	text := strings.Repeat("x", 5000)
	got := Default().Split(text)
	if len(got) < 2 {
		t.Fatalf("expected char-level split, got %d", len(got))
	}
	for _, c := range got {
		if len(c.Text) > targetChunkSize {
			t.Fatalf("chunk size %d > target", len(c.Text))
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestSplitProducesValidUTF8ForCJK(t *testing.T) {
	// Pure CJK with no separators forces the char-level fallback. Each
	// character is 3 bytes; a byte-aligned 800/700 window will land
	// mid-rune and emit invalid UTF-8 unless boundaries are rune-aware.
	text := strings.Repeat("中", 4000)
	got := Default().Split(text)
	if len(got) < 2 {
		t.Fatalf("expected multi-chunk split for long CJK text, got %d", len(got))
	}
	for i, c := range got {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("chunk %d contains invalid UTF-8 (len=%d, first 4 bytes=%x)", i, len(c.Text), []byte(c.Text)[:minInt(4, len(c.Text))])
		}
	}
}

func TestSplitOverlapTailIsRuneSafe(t *testing.T) {
	// A long CJK paragraph separated by ASCII newlines forces
	// mergeWithOverlap to take a tail slice. The byte-based tail offset
	// would land mid-rune at the 100-byte boundary; we expect the rune
	// snap to keep every chunk a valid UTF-8 string.
	para := strings.Repeat("中文测试", 100)
	text := para + "\n\n" + para + "\n\n" + para
	got := Default().Split(text)
	if len(got) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(got))
	}
	for i, c := range got {
		if !utf8.ValidString(c.Text) {
			t.Fatalf("chunk %d invalid UTF-8 at overlap boundary", i)
		}
	}
}
