package queue

import (
"context"
"log"
"time"

"github.com/mauludsadiq/agent-control-plane/acpd/internal/store"
)

type Queue struct {
db *store.DB
}

func New(db *store.DB) *Queue {
return &Queue{db: db}
}

// RequeueLoop runs in the background, periodically:
// 1. Requeuing tasks whose workers have timed out without heartbeating
// 2. Dead-lettering tasks that have exhausted max_attempts
func (q *Queue) RequeueLoop(ctx context.Context, interval time.Duration) {
ticker := time.NewTicker(interval)
defer ticker.Stop()
for {
select {
case <-ctx.Done():
return
case <-ticker.C:
requeued, deadLettered, err := q.db.RequeueExpiredTasksWithDeadLetter()
if err != nil {
log.Printf("requeue: %v", err)
} else if requeued > 0 || deadLettered > 0 {
log.Printf("requeue: %d requeued, %d dead-lettered", requeued, deadLettered)
}
}
}
}
