package interview

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu                    sync.RWMutex
	interviews            map[string]Interview
	tokenToInterview      map[string]string
	rescheduleRequests    map[string]RescheduleRequest
	processRecords        map[string][]ProcessRecord
	notificationLog       []NotificationEvent
	nextInterview         int64
	nextEvent             int64
	nextRescheduleRequest int64
	nextProcessRecord     int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		interviews:         make(map[string]Interview),
		tokenToInterview:   make(map[string]string),
		rescheduleRequests: make(map[string]RescheduleRequest),
		processRecords:     make(map[string][]ProcessRecord),
	}
}

func (r *MemoryRepository) CreateInterview(item Interview) Interview {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextInterview++
	now := time.Now().UTC()
	item.ID = fmt.Sprintf("int_%06d", r.nextInterview)
	if item.CandidateToken == "" {
		item.CandidateToken = fmt.Sprintf("candtok_%06d", r.nextInterview)
	}
	if item.CandidateState == "" {
		item.CandidateState = CandidateResponseAwaiting
	}
	item.CreatedAt = now
	item.UpdatedAt = now
	item.InterviewerIDs = append([]string(nil), item.InterviewerIDs...)
	r.interviews[item.ID] = item
	r.tokenToInterview[item.CandidateToken] = item.ID
	return cloneInterview(item)
}

func (r *MemoryRepository) UpdateInterview(item Interview) Interview {
	r.mu.Lock()
	defer r.mu.Unlock()

	item.UpdatedAt = time.Now().UTC()
	item.InterviewerIDs = append([]string(nil), item.InterviewerIDs...)
	if item.CandidateToken != "" {
		r.tokenToInterview[item.CandidateToken] = item.ID
	}
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

func (r *MemoryRepository) GetInterviewByCandidateToken(token string) (Interview, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	interviewID, ok := r.tokenToInterview[token]
	if !ok {
		return Interview{}, false
	}
	item, ok := r.interviews[interviewID]
	if !ok {
		return Interview{}, false
	}
	return cloneInterview(item), true
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

func (r *MemoryRepository) SaveRescheduleRequest(item RescheduleRequest) RescheduleRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	if item.ID == "" {
		r.nextRescheduleRequest++
		item.ID = fmt.Sprintf("rr_%06d", r.nextRescheduleRequest)
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	r.rescheduleRequests[item.InterviewID] = item
	return cloneRescheduleRequest(item)
}

func (r *MemoryRepository) GetRescheduleRequest(interviewID string) (RescheduleRequest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.rescheduleRequests[interviewID]
	if !ok {
		return RescheduleRequest{}, false
	}
	return cloneRescheduleRequest(item), true
}

func (r *MemoryRepository) AddProcessRecord(record ProcessRecord) ProcessRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextProcessRecord++
	record.ID = fmt.Sprintf("pr_%06d", r.nextProcessRecord)
	record.CreatedAt = time.Now().UTC()
	r.processRecords[record.InterviewID] = append(r.processRecords[record.InterviewID], record)
	return cloneProcessRecord(record)
}

func (r *MemoryRepository) ListProcessRecords(interviewID string) []ProcessRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := r.processRecords[interviewID]
	out := make([]ProcessRecord, 0, len(items))
	for _, item := range items {
		out = append(out, cloneProcessRecord(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func cloneInterview(in Interview) Interview {
	out := in
	out.InterviewerIDs = append([]string(nil), in.InterviewerIDs...)
	return out
}

func cloneRescheduleRequest(in RescheduleRequest) RescheduleRequest {
	return in
}

func cloneProcessRecord(in ProcessRecord) ProcessRecord {
	return in
}
