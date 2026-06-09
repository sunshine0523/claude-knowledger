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
