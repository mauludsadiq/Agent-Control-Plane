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

// RequeueLoop runs in the background, periodically requeuing tasks whose
// workers have timed out without completing.
func (q *Queue) RequeueLoop(ctx context.Context, interval time.Duration) {
ticker := time.NewTicker(interval)
defer ticker.Stop()
for {
select {
case <-ctx.Done():
return
case <-ticker.C:
n, err := q.db.RequeueExpiredTasks()
if err != nil {
log.Printf("requeue: %v", err)
} else if n > 0 {
log.Printf("requeued %d expired tasks", n)
}
}
}
}
