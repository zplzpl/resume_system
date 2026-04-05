package interview

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu                sync.RWMutex
	interviews        map[string]Interview
	evaluationArchive map[string][]Evaluation
	questionBank      map[string]QuestionRecommendation
	notificationLog   []NotificationEvent
	nextInterview     int64
	nextEvent         int64
	nextEvaluation    int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		interviews:        make(map[string]Interview),
		evaluationArchive: make(map[string][]Evaluation),
		questionBank:      make(map[string]QuestionRecommendation),
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

func (r *MemoryRepository) AddEvaluation(item Evaluation) Evaluation {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	archive := r.evaluationArchive[item.InterviewID]
	version := 1
	for i := range archive {
		if archive[i].InterviewerID != item.InterviewerID {
			continue
		}
		version++
		if archive[i].IsLatest {
			archivedAt := now
			archive[i].IsLatest = false
			archive[i].ArchivedAt = &archivedAt
		}
	}

	r.nextEvaluation++
	item.ID = fmt.Sprintf("eval_%06d", r.nextEvaluation)
	item.Version = version
	item.SubmittedAt = now
	item.IsLatest = true
	item.ArchivedAt = nil
	item.CapabilityScores = cloneCapabilityScores(item.CapabilityScores)

	archive = append(archive, item)
	r.evaluationArchive[item.InterviewID] = archive
	return cloneEvaluation(item)
}

func (r *MemoryRepository) ListEvaluationsByInterview(interviewID string) []Evaluation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	archive := r.evaluationArchive[interviewID]
	out := make([]Evaluation, 0, len(archive))
	for _, item := range archive {
		out = append(out, cloneEvaluation(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SubmittedAt.Equal(out[j].SubmittedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].SubmittedAt.After(out[j].SubmittedAt)
	})
	return out
}

func (r *MemoryRepository) ListLatestEvaluationsByCandidate(candidateID string) []Evaluation {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Evaluation, 0)
	for _, archive := range r.evaluationArchive {
		for _, item := range archive {
			if item.CandidateID != candidateID || !item.IsLatest {
				continue
			}
			out = append(out, cloneEvaluation(item))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SubmittedAt.Equal(out[j].SubmittedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].SubmittedAt.After(out[j].SubmittedAt)
	})
	return out
}

func (r *MemoryRepository) SaveQuestionRecommendation(item QuestionRecommendation) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.questionBank[item.InterviewID] = cloneQuestionRecommendation(item)
}

func (r *MemoryRepository) GetQuestionRecommendation(interviewID string) (QuestionRecommendation, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.questionBank[interviewID]
	if !ok {
		return QuestionRecommendation{}, false
	}
	return cloneQuestionRecommendation(item), true
}

func cloneInterview(in Interview) Interview {
	out := in
	out.InterviewerIDs = append([]string(nil), in.InterviewerIDs...)
	return out
}

func cloneEvaluation(in Evaluation) Evaluation {
	out := in
	out.CapabilityScores = cloneCapabilityScores(in.CapabilityScores)
	if in.ArchivedAt != nil {
		archivedAt := *in.ArchivedAt
		out.ArchivedAt = &archivedAt
	}
	return out
}

func cloneCapabilityScores(in []CapabilityScore) []CapabilityScore {
	return append([]CapabilityScore(nil), in...)
}

func cloneQuestionRecommendation(in QuestionRecommendation) QuestionRecommendation {
	out := in
	out.Questions = append([]RecommendedQuestion(nil), in.Questions...)
	return out
}
