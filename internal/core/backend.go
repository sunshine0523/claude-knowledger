package core

import "context"

type ScopedKBRef struct {
	Scope string
	ID    string
}

type SearchOptions struct {
	Query      string
	Limit      int
	KBIDs      []ScopedKBRef
	SearchMode string
}

type AddInput struct {
	KBID     string
	Scope    string
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
	Rebuild  bool          `json:"rebuild"`
	Progress IndexProgress `json:"-"`
}

// IndexProgressPhase identifies the kind of progress event emitted while
// MaintainIndex runs. Concrete strings are stable; consumers may match on them
// in switch statements.
type IndexProgressPhase string

const (
	IndexProgressPhaseStart        IndexProgressPhase = "start"
	IndexProgressPhaseRebuildReset IndexProgressPhase = "rebuild_reset"
	IndexProgressPhaseIndex        IndexProgressPhase = "index"
	IndexProgressPhaseSkip         IndexProgressPhase = "skip"
	IndexProgressPhaseDeleteOrphan IndexProgressPhase = "delete_orphan"
	IndexProgressPhaseDone         IndexProgressPhase = "done"
)

// IndexProgressEvent is a single observation emitted by MaintainIndex.
// Done is the count of items considered (indexed+skipped) so far; Total is the
// number known up-front (0 if not yet known). Item is the current item id when
// applicable. Message is a human-readable hint, optional.
type IndexProgressEvent struct {
	KBID    string
	Phase   IndexProgressPhase
	Item    string
	Done    int
	Total   int
	Message string
}

type IndexProgress func(IndexProgressEvent)

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
