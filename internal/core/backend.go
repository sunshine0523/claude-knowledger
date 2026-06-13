package core

import "context"

type SearchOptions struct {
	Query      string
	Limit      int
	KBIDs      []string
	SearchMode string
}

type AddInput struct {
	KBID     string
	Title    string
	Content  string
	Tags     []string
	Metadata map[string]any
}

type StoreBackend interface {
	Add(context.Context, KnowledgeBase, AddInput) (KnowledgeItem, IngestionResult, IndexStatus, error)
	Search(context.Context, KnowledgeBase, SearchOptions) ([]SearchHit, error)
	GetItem(context.Context, KnowledgeBase, string) (KnowledgeItem, error)
	ListItems(context.Context, KnowledgeBase) ([]KnowledgeItem, error)
	DeleteItem(context.Context, KnowledgeBase, string) error
	SupportsSemantic(KnowledgeBase) bool
}

type IndexOptions struct {
	Rebuild bool `json:"rebuild"`
}

type IndexResult struct {
	Indexed  int      `json:"indexed"`
	Deleted  int      `json:"deleted"`
	Skipped  int      `json:"skipped"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

type IndexMaintainer interface {
	MaintainIndex(context.Context, KnowledgeBase, IndexOptions) (IndexResult, error)
}
