package indexing

import "context"

type Worker struct {
	queue   *MemoryQueue
	process func(context.Context, Job) error
}

func NewWorker(queue *MemoryQueue, process func(context.Context, Job) error) *Worker {
	return &Worker{queue: queue, process: process}
}

func (w *Worker) ProcessOne(ctx context.Context) error {
	for i := range w.queue.jobs {
		if w.queue.jobs[i].State != "queued" {
			continue
		}
		w.queue.jobs[i].State = "indexing"
		if err := w.process(ctx, w.queue.jobs[i]); err != nil {
			w.queue.jobs[i].State = "failed"
			w.queue.jobs[i].Error = err.Error()
			return err
		}
		w.queue.jobs[i].State = "indexed"
		return nil
	}
	return nil
}
