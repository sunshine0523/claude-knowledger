package indexing_test

import (
	"context"
	"testing"
	"time"

	"github.com/kindbrave/knowledger/internal/indexing"
)

func TestWorkerMarksQueuedJobIndexed(t *testing.T) {
	queue := indexing.NewMemoryQueue()
	queue.Enqueue(indexing.Job{KBID: "notes", ItemID: "1"})

	worker := indexing.NewWorker(queue, func(context.Context, indexing.Job) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne returned error: %v", err)
	}

	job := queue.Jobs()[0]
	if job.State != "indexed" {
		t.Fatalf("expected indexed state, got %q", job.State)
	}
}
