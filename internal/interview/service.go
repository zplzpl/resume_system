package interview

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInterviewNotFound = errors.New("interview not found")
)

type ConflictError struct {
	Conflicts []Conflict
}

func (e *ConflictError) Error() string {
	if len(e.Conflicts) == 0 {
		return "interview schedule conflicts"
	}
	return fmt.Sprintf("interview schedule conflicts: %d", len(e.Conflicts))
}

type CreateRequest struct {
	CandidateID    string
	InterviewerIDs []string
	StartsAt       time.Time
	EndsAt         time.Time
	Round          string
	Note           string
	CreatedBy      string
}

type UpdateRequest struct {
	CandidateID    *string
	InterviewerIDs *[]string
	StartsAt       *time.Time
	EndsAt         *time.Time
	Round          *string
	Status         *string
	Note           *string
}

type OperationResult struct {
	Interview             Interview           `json:"interview"`
	Notifications         []NotificationEvent `json:"notifications"`
	NotificationsEnqueued int                 `json:"notifications_enqueued"`
}

type Service struct {
	repo *MemoryRepository
}

func NewService(repo *MemoryRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(req CreateRequest) (OperationResult, error) {
	candidateID := strings.TrimSpace(req.CandidateID)
	if candidateID == "" {
		return OperationResult{}, fmt.Errorf("candidate_id is required")
	}
	interviewerIDs := dedupNonEmpty(req.InterviewerIDs)
	if len(interviewerIDs) == 0 {
		return OperationResult{}, fmt.Errorf("interviewer_ids is required")
	}
	if !req.StartsAt.Before(req.EndsAt) {
		return OperationResult{}, fmt.Errorf("starts_at must be before ends_at")
	}

	item := Interview{
		CandidateID:    candidateID,
		InterviewerIDs: interviewerIDs,
		Round:          normalizeRound(req.Round),
		Status:         StatusScheduled,
		StartsAt:       req.StartsAt.UTC(),
		EndsAt:         req.EndsAt.UTC(),
		Note:           strings.TrimSpace(req.Note),
		CreatedBy:      strings.TrimSpace(req.CreatedBy),
	}
	item.ColorTag = ComputeColorTag(item.Round, item.Status)

	if conflicts := s.detectConflicts(item, ""); len(conflicts) > 0 {
		return OperationResult{}, &ConflictError{Conflicts: conflicts}
	}

	created := s.repo.CreateInterview(item)
	events := s.repo.EnqueueNotifications(buildNotificationEvents(created, "interview.created"))
	return OperationResult{
		Interview:             created,
		Notifications:         events,
		NotificationsEnqueued: len(events),
	}, nil
}

func (s *Service) Update(id string, req UpdateRequest) (OperationResult, error) {
	id = strings.TrimSpace(id)
	item, ok := s.repo.GetInterview(id)
	if !ok {
		return OperationResult{}, fmt.Errorf("%w: %s", ErrInterviewNotFound, id)
	}

	timeChanged := false
	if req.CandidateID != nil {
		candidateID := strings.TrimSpace(*req.CandidateID)
		if candidateID == "" {
			return OperationResult{}, fmt.Errorf("candidate_id cannot be empty")
		}
		item.CandidateID = candidateID
	}
	if req.InterviewerIDs != nil {
		interviewerIDs := dedupNonEmpty(*req.InterviewerIDs)
		if len(interviewerIDs) == 0 {
			return OperationResult{}, fmt.Errorf("interviewer_ids cannot be empty")
		}
		item.InterviewerIDs = interviewerIDs
	}
	if req.StartsAt != nil {
		item.StartsAt = req.StartsAt.UTC()
		timeChanged = true
	}
	if req.EndsAt != nil {
		item.EndsAt = req.EndsAt.UTC()
		timeChanged = true
	}
	if !item.StartsAt.Before(item.EndsAt) {
		return OperationResult{}, fmt.Errorf("starts_at must be before ends_at")
	}
	if req.Round != nil {
		item.Round = normalizeRound(*req.Round)
	}
	if req.Note != nil {
		item.Note = strings.TrimSpace(*req.Note)
	}
	if req.Status != nil {
		status, err := ParseStatus(*req.Status)
		if err != nil {
			return OperationResult{}, err
		}
		item.Status = status
	} else if timeChanged && item.Status == StatusScheduled {
		item.Status = StatusRescheduled
	}

	item.ColorTag = ComputeColorTag(item.Round, item.Status)
	if conflicts := s.detectConflicts(item, item.ID); len(conflicts) > 0 {
		return OperationResult{}, &ConflictError{Conflicts: conflicts}
	}

	updated := s.repo.UpdateInterview(item)
	events := s.repo.EnqueueNotifications(buildNotificationEvents(updated, "interview.updated"))
	return OperationResult{
		Interview:             updated,
		Notifications:         events,
		NotificationsEnqueued: len(events),
	}, nil
}

func (s *Service) Calendar(view CalendarView, anchor time.Time) CalendarResult {
	anchor = anchor.UTC()
	rangeFrom, rangeTo := calendarRange(view, anchor)
	all := s.repo.ListInterviews()

	items := make([]Interview, 0, len(all))
	for _, item := range all {
		if overlaps(item.StartsAt, item.EndsAt, rangeFrom, rangeTo) {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].StartsAt.Equal(items[j].StartsAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].StartsAt.Before(items[j].StartsAt)
	})

	return CalendarResult{
		View:      view,
		RangeFrom: rangeFrom,
		RangeTo:   rangeTo,
		Items:     items,
	}
}

func IsConflictError(err error) bool {
	var conflictErr *ConflictError
	return errors.As(err, &conflictErr)
}

func IsInterviewNotFound(err error) bool {
	return errors.Is(err, ErrInterviewNotFound)
}

func ExtractConflictError(err error) *ConflictError {
	var conflictErr *ConflictError
	if errors.As(err, &conflictErr) {
		return conflictErr
	}
	return nil
}

func (s *Service) detectConflicts(target Interview, excludeID string) []Conflict {
	all := s.repo.ListInterviews()
	conflicts := make([]Conflict, 0)

	for _, item := range all {
		if item.ID == excludeID || !IsActiveStatus(item.Status) {
			continue
		}
		if !overlaps(target.StartsAt, target.EndsAt, item.StartsAt, item.EndsAt) {
			continue
		}

		if item.CandidateID == target.CandidateID {
			conflicts = append(conflicts, Conflict{
				Type:         "candidate_time_conflict",
				InterviewID:  item.ID,
				EntityType:   "candidate",
				EntityID:     target.CandidateID,
				ConflictFrom: maxTime(target.StartsAt, item.StartsAt),
				ConflictTo:   minTime(target.EndsAt, item.EndsAt),
				Message:      "candidate has overlapping interview",
			})
		}

		sharedInterviewers := intersect(target.InterviewerIDs, item.InterviewerIDs)
		for _, interviewerID := range sharedInterviewers {
			conflicts = append(conflicts, Conflict{
				Type:         "interviewer_time_conflict",
				InterviewID:  item.ID,
				EntityType:   "interviewer",
				EntityID:     interviewerID,
				ConflictFrom: maxTime(target.StartsAt, item.StartsAt),
				ConflictTo:   minTime(target.EndsAt, item.EndsAt),
				Message:      "interviewer has overlapping interview",
			})
		}
	}

	return conflicts
}

func buildNotificationEvents(item Interview, eventType string) []NotificationEvent {
	events := make([]NotificationEvent, 0, (1+len(item.InterviewerIDs))*2)
	channels := []string{"in_app", "email"}

	for _, channel := range channels {
		events = append(events, NotificationEvent{
			InterviewID:   item.ID,
			RecipientID:   item.CandidateID,
			RecipientType: "candidate",
			Channel:       channel,
			EventType:     eventType,
		})
	}

	for _, interviewerID := range item.InterviewerIDs {
		for _, channel := range channels {
			events = append(events, NotificationEvent{
				InterviewID:   item.ID,
				RecipientID:   interviewerID,
				RecipientType: "interviewer",
				Channel:       channel,
				EventType:     eventType,
			})
		}
	}

	return events
}

func calendarRange(view CalendarView, anchor time.Time) (time.Time, time.Time) {
	base := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)
	switch view {
	case CalendarViewDay:
		return base, base.AddDate(0, 0, 1)
	case CalendarViewMonth:
		start := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	default:
		weekday := int(base.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := base.AddDate(0, 0, -(weekday - 1))
		return start, start.AddDate(0, 0, 7)
	}
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}

func dedupNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intersect(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	lookup := make(map[string]struct{}, len(left))
	for _, value := range left {
		lookup[value] = struct{}{}
	}
	out := make([]string, 0)
	for _, value := range right {
		if _, ok := lookup[value]; ok {
			out = append(out, value)
		}
	}
	return dedupNonEmpty(out)
}

func normalizeRound(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "round-1"
	}
	return raw
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
