package resume

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu               sync.RWMutex
	resumes          map[string]ResumeRecord
	candidates       map[string]CandidateProfile
	nextResumeSeq    int64
	nextCandidateSeq int64
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		resumes:    make(map[string]ResumeRecord),
		candidates: make(map[string]CandidateProfile),
	}
}

func (r *MemoryRepository) CreateResume(record ResumeRecord) ResumeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextResumeSeq++
	record.ID = fmt.Sprintf("res_%06d", r.nextResumeSeq)
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	r.resumes[record.ID] = cloneResume(record)
	return cloneResume(record)
}

func (r *MemoryRepository) UpdateResume(record ResumeRecord) ResumeRecord {
	r.mu.Lock()
	defer r.mu.Unlock()

	record.UpdatedAt = time.Now().UTC()
	r.resumes[record.ID] = cloneResume(record)
	return cloneResume(record)
}

func (r *MemoryRepository) GetResume(id string) (ResumeRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	record, ok := r.resumes[id]
	if !ok {
		return ResumeRecord{}, false
	}
	return cloneResume(record), true
}

func (r *MemoryRepository) UpsertCandidate(candidateID string, parsed ParsedCoreFields, sourceResumeID string) CandidateProfile {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	var candidate CandidateProfile
	if candidateID != "" {
		candidate = r.candidates[candidateID]
	}
	if candidate.ID == "" {
		r.nextCandidateSeq++
		candidate.ID = fmt.Sprintf("cand_%06d", r.nextCandidateSeq)
		candidate.CreatedAt = now
	}
	candidate.UpdatedAt = now
	candidate.FullName = chooseValue(parsed.FullName, candidate.FullName, "Unknown Candidate")
	candidate.Email = chooseValue(parsed.Email, candidate.Email, "")
	candidate.Phone = chooseValue(parsed.Phone, candidate.Phone, "")
	candidate.CurrentCompany = chooseValue(parsed.CurrentCompany, candidate.CurrentCompany, "")
	candidate.CurrentTitle = chooseValue(parsed.CurrentTitle, candidate.CurrentTitle, "")
	candidate.HighestEducation = chooseValue(parsed.HighestEducation, candidate.HighestEducation, "")
	candidate.Location = chooseValue(parsed.Location, candidate.Location, "")
	if parsed.TotalExperienceMonths > 0 {
		candidate.TotalExperienceMonths = parsed.TotalExperienceMonths
	}
	if len(parsed.Skills) > 0 {
		candidate.Skills = append([]string(nil), parsed.Skills...)
	}
	candidate.SourceResumeID = sourceResumeID
	r.candidates[candidate.ID] = cloneCandidate(candidate)
	return cloneCandidate(candidate)
}

func (r *MemoryRepository) CreateCandidate(name, email, phone string) CandidateProfile {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextCandidateSeq++
	now := time.Now().UTC()
	candidate := CandidateProfile{
		ID:        fmt.Sprintf("cand_%06d", r.nextCandidateSeq),
		FullName:  chooseValue(name, "", "Unknown Candidate"),
		Email:     email,
		Phone:     phone,
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.candidates[candidate.ID] = cloneCandidate(candidate)
	return cloneCandidate(candidate)
}

func (r *MemoryRepository) ListCandidates() []CandidateProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]CandidateProfile, 0, len(r.candidates))
	for _, candidate := range r.candidates {
		items = append(items, cloneCandidate(candidate))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items
}

func cloneResume(in ResumeRecord) ResumeRecord {
	out := in
	out.ParsedPayload.Skills = append([]string(nil), in.ParsedPayload.Skills...)
	return out
}

func cloneCandidate(in CandidateProfile) CandidateProfile {
	out := in
	out.Skills = append([]string(nil), in.Skills...)
	return out
}

func chooseValue(primary, fallback, defaultValue string) string {
	if primary != "" {
		return primary
	}
	if fallback != "" {
		return fallback
	}
	return defaultValue
}
