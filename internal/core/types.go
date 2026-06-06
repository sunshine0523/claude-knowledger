package core

import "time"

type KnowledgeBase struct {
	ID                string
	Name              string
	StoreType         string
	StoreConfig       map[string]any
	Enabled           bool
	DefaultSearchMode string
	Indexing          map[string]any
	Tags              []string
}

type KnowledgeItem struct {
	ID        string
	KBID      string
	Type      string
	Title     string
	Content   string
	Summary   string
	SourceRef string
	Metadata  map[string]any
	Tags      []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SearchHit struct {
	ItemID         string
	KBID           string
	ItemType       string
	Title          string
	Snippet        string
	ContentPreview string
	Score          float64
	MatchMode      string
	SourceBackend  string
	Locator        string
	Metadata       map[string]any
}

type IngestionResult struct {
	Success     bool
	ItemID      string
	IndexQueued bool
	Warnings    []string
}

type IndexStatus struct {
	State         string
	LastSuccessAt *time.Time
	LastError     string
}
