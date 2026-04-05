package interview

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu              sync.RWMutex
	interviews      map[string]Interview
	notificationLog []NotificationEvent
	nextInterview   int64
	nextEvent       int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		interviews: make(map[string]Interview),
	}
}

func (r *MemoryRepository) CreateInterview(item Interview) Interview {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextInterview++
	now := time.Now().UTC()
	item.ID = fmt.Sprintf("int_%06d", r.nextInterview)
	item.CreatedAt = now
	item.UpdatedAt = now
	item.InterviewerIDs = append([]string(nil), item.InterviewerIDs...)
	r.interviews[item.ID] = item
	return cloneInterview(item)
}

func (r *MemoryRepository) UpdateInterview(item Interview) Interview {
	r.mu.Lock()
	defer r.mu.Unlock()

	item.UpdatedAt = time.Now().UTC()
	item.InterviewerIDs = append([]string(nil), item.InterviewerIDs...)
	r.interviews[item.ID] = item
	return cloneInterview(item)
}

func (r *MemoryRepository) GetInterview(id string) (Interview, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.interviews[id]
	if !ok {
		return Interview{}, false
	}
	return cloneInterview(item), true
}

func (r *MemoryRepository) ListInterviews() []Interview {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Interview, 0, len(r.interviews))
	for _, item := range r.interviews {
		items = append(items, cloneInterview(item))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].StartsAt.Equal(items[j].StartsAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].StartsAt.Before(items[j].StartsAt)
	})
	return items
}

func (r *MemoryRepository) EnqueueNotifications(events []NotificationEvent) []NotificationEvent {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]NotificationEvent, 0, len(events))
	for _, event := range events {
		r.nextEvent++
		event.ID = fmt.Sprintf("evt_%06d", r.nextEvent)
		event.CreatedAt = time.Now().UTC()
		r.notificationLog = append(r.notificationLog, event)
		out = append(out, event)
	}
	return out
}

func cloneInterview(in Interview) Interview {
	out := in
	out.InterviewerIDs = append([]string(nil), in.InterviewerIDs...)
	return out
}
