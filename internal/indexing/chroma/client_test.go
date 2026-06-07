package chroma

import "testing"

func TestConfigEffectiveModeDefaultsToPersistent(t *testing.T) {
	cfg := Config{}

	if got := cfg.EffectiveMode(); got != ModePersistent {
		t.Fatalf("EffectiveMode() = %q, want %q", got, ModePersistent)
	}
}

func TestNormalizeLimitDefaultsNonPositiveValues(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "negative limit", limit: -1, want: 10},
		{name: "zero limit", limit: 0, want: 10},
		{name: "positive limit", limit: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLimit(tt.limit); got != tt.want {
				t.Fatalf("normalizeLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

func TestScoreFromDistance(t *testing.T) {
	tests := []struct {
		name     string
		distance float64
		want     float64
	}{
		{name: "zero distance", distance: 0, want: 1},
		{name: "unit distance", distance: 1, want: 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ScoreFromDistance(tt.distance); got != tt.want {
				t.Fatalf("ScoreFromDistance(%v) = %v, want %v", tt.distance, got, tt.want)
			}
		})
	}
}

func TestHitTitleReturnsMetadataTitle(t *testing.T) {
	hit := Hit{Metadata: map[string]any{"title": "Core notes"}}

	if got := hit.Title(); got != "Core notes" {
		t.Fatalf("Title() = %q, want %q", got, "Core notes")
	}
}
