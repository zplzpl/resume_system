package audit

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultQueryLimit = 100
	maxQueryLimit     = 1000
)

type MemoryRepository struct {
	mu      sync.RWMutex
	events  []Event
	nextSeq int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{}
}

func (r *MemoryRepository) Add(input RecordInput) Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextSeq++
	event := Event{
		ID:         fmt.Sprintf("audit_%06d", r.nextSeq),
		ActionType: strings.TrimSpace(input.ActionType),
		OperatorID: strings.TrimSpace(input.OperatorID),
		ObjectType: strings.TrimSpace(input.ObjectType),
		ObjectID:   strings.TrimSpace(input.ObjectID),
		OccurredAt: time.Now().UTC(),
		Metadata:   cloneMetadata(input.Metadata),
	}
	r.events = append(r.events, event)
	return cloneEvent(event)
}

func (r *MemoryRepository) Query(filter QueryFilter) []Event {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := normalizeLimit(filter.Limit)
	items := make([]Event, 0, limit)
	for i := len(r.events) - 1; i >= 0; i-- {
		item := r.events[i]
		if !matchesFilter(item, filter) {
			continue
		}
		items = append(items, cloneEvent(item))
		if len(items) >= limit {
			break
		}
	}
	return items
}

func normalizeLimit(raw int) int {
	switch {
	case raw <= 0:
		return defaultQueryLimit
	case raw > maxQueryLimit:
		return maxQueryLimit
	default:
		return raw
	}
}

func matchesFilter(item Event, filter QueryFilter) bool {
	if filter.ActionType != "" && item.ActionType != strings.TrimSpace(filter.ActionType) {
		return false
	}
	if filter.OperatorID != "" && item.OperatorID != strings.TrimSpace(filter.OperatorID) {
		return false
	}
	if filter.ObjectType != "" && item.ObjectType != strings.TrimSpace(filter.ObjectType) {
		return false
	}
	if filter.ObjectID != "" && item.ObjectID != strings.TrimSpace(filter.ObjectID) {
		return false
	}
	if filter.From != nil && item.OccurredAt.Before(filter.From.UTC()) {
		return false
	}
	if filter.To != nil && item.OccurredAt.After(filter.To.UTC()) {
		return false
	}
	return true
}

func cloneEvent(in Event) Event {
	out := in
	out.Metadata = cloneMetadata(in.Metadata)
	return out
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
