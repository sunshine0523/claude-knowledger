package indexing

type Job struct {
	KBID   string
	ItemID string
	State  string
	Error  string
}

type MemoryQueue struct {
	jobs []Job
}

func NewMemoryQueue() *MemoryQueue { return &MemoryQueue{} }

func (q *MemoryQueue) Enqueue(job Job) {
	job.State = "queued"
	q.jobs = append(q.jobs, job)
}

func (q *MemoryQueue) Jobs() []Job {
	out := make([]Job, len(q.jobs))
	copy(out, q.jobs)
	return out
}
