package resume

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

type Service struct {
	repo    *MemoryRepository
	storage *LocalStorage
	parser  *HeuristicParser
}

type UploadResult struct {
	Resume    ResumeRecord      `json:"resume"`
	Candidate *CandidateProfile `json:"candidate,omitempty"`
}

func NewService(repo *MemoryRepository, storage *LocalStorage, parser *HeuristicParser) *Service {
	return &Service{repo: repo, storage: storage, parser: parser}
}

func (s *Service) Upload(fileName string, source io.Reader, uploaderID, candidateID string) (UploadResult, error) {
	stored, err := s.storage.Save(fileName, source)
	if err != nil {
		return UploadResult{}, fmt.Errorf("store upload: %w", err)
	}

	resume := s.repo.CreateResume(ResumeRecord{
		CandidateID: candidateID,
		FileName:    fileName,
		ContentType: stored.ContentType,
		FileSize:    stored.Size,
		FileHash:    stored.Hash,
		StoragePath: stored.StoragePath,
		ParseStatus: ParseStatusPending,
		UploadedBy:  uploaderID,
	})

	if !IsSupportedFileName(fileName) {
		resume.ParseStatus = ParseStatusFailed
		resume.FailureReason = ErrUnsupportedFormat.Error()
		resume = s.repo.UpdateResume(resume)
		return UploadResult{Resume: resume}, nil
	}

	return s.parseAndPersist(resume)
}

func (s *Service) Retry(id string) (UploadResult, error) {
	resume, ok := s.repo.GetResume(id)
	if !ok {
		return UploadResult{}, fmt.Errorf("resume %s not found", id)
	}
	if resume.ParseStatus != ParseStatusFailed {
		return UploadResult{}, fmt.Errorf("resume %s is not in failed status", id)
	}
	if resume.StoragePath == "" {
		return UploadResult{}, fmt.Errorf("resume %s has empty storage path", id)
	}
	return s.parseAndPersist(resume)
}

func (s *Service) GetResume(id string) (UploadResult, bool) {
	resume, ok := s.repo.GetResume(id)
	if !ok {
		return UploadResult{}, false
	}
	result := UploadResult{Resume: resume}
	if resume.CandidateID != "" {
		candidate := s.findCandidate(resume.CandidateID)
		if candidate != nil {
			result.Candidate = candidate
		}
	}
	return result, true
}

func (s *Service) ListCandidates() []CandidateProfile {
	return s.SearchCandidates(CandidateSearchOptions{})
}

func (s *Service) SearchCandidates(options CandidateSearchOptions) []CandidateProfile {
	options.Keyword = strings.TrimSpace(options.Keyword)
	options.Skill = strings.TrimSpace(options.Skill)
	options.Company = strings.TrimSpace(options.Company)
	options.School = strings.TrimSpace(options.School)
	return s.repo.SearchCandidates(options)
}

func (s *Service) CreateManualCandidate(name, email, phone string) CandidateProfile {
	return s.repo.CreateCandidate(strings.TrimSpace(name), strings.TrimSpace(email), strings.TrimSpace(phone))
}

func (s *Service) CandidateExists(candidateID string) bool {
	_, ok := s.repo.GetCandidate(strings.TrimSpace(candidateID))
	return ok
}

func (s *Service) UpdateCandidateStatusLayer(candidateID, statusRaw string) (CandidateProfile, error) {
	status, err := ParseCandidateStatusLayer(statusRaw)
	if err != nil {
		return CandidateProfile{}, err
	}

	candidate, ok := s.repo.UpdateCandidateStatusLayer(strings.TrimSpace(candidateID), status)
	if !ok {
		return CandidateProfile{}, fmt.Errorf("%w: %s", ErrCandidateNotFound, candidateID)
	}
	return candidate, nil
}

func (s *Service) parseAndPersist(resume ResumeRecord) (UploadResult, error) {
	resume.ParseStatus = ParseStatusProcessing
	resume.FailureReason = ""
	resume = s.repo.UpdateResume(resume)

	parsed, err := s.parser.Parse(resume.StoragePath)
	if err != nil {
		resume.ParseStatus = ParseStatusFailed
		resume.FailureReason = err.Error()
		resume.ParsedPayload = ParsedCoreFields{}
		resume.ParsedAt = nil
		resume = s.repo.UpdateResume(resume)
		return UploadResult{Resume: resume}, nil
	}

	candidate := s.repo.UpsertCandidate(resume.CandidateID, parsed, resume.ID)
	now := time.Now().UTC()
	resume.CandidateID = candidate.ID
	resume.ParseStatus = ParseStatusSuccess
	resume.FailureReason = ""
	resume.ParsedPayload = parsed
	resume.ParsedAt = &now
	resume = s.repo.UpdateResume(resume)

	candidateCopy := candidate
	return UploadResult{Resume: resume, Candidate: &candidateCopy}, nil
}

func (s *Service) findCandidate(candidateID string) *CandidateProfile {
	candidate, ok := s.repo.GetCandidate(candidateID)
	if !ok {
		return nil
	}
	copy := candidate
	return &copy
}

func IsCandidateNotFound(err error) bool {
	return errors.Is(err, ErrCandidateNotFound)
}

func IsInvalidStatusLayer(err error) bool {
	return errors.Is(err, ErrInvalidStatusLayer)
}
