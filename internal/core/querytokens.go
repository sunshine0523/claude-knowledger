package core

import "unicode"

// TokenizeQuery splits a search query into OR-able tokens.
// Separators are runes for which unicode.IsSpace OR unicode.IsPunct holds
// (covers ASCII whitespace, fullwidth space, ASCII punctuation, CJK
// punctuation). Word characters preserved include letters in any script,
// digits, and the underscore. Hyphens and other dash punctuation are
// separators (so "user-id" splits but "user_id" stays whole).
//
// The result preserves first-occurrence order and is de-duplicated.
// Returns nil for empty / separator-only input.
func TokenizeQuery(q string) []string {
	if q == "" {
		return nil
	}
	var (
		out  []string
		seen = map[string]struct{}{}
		buf  []rune
	)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		tok := string(buf)
		buf = buf[:0]
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	for _, r := range q {
		if r == '_' {
			buf = append(buf, r)
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			flush()
			continue
		}
		buf = append(buf, r)
	}
	flush()
	return out
}
