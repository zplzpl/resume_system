package interview

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInterviewNotFound         = errors.New("interview not found")
	ErrCandidateTokenNotFound    = errors.New("candidate token not found")
	ErrNoPendingRescheduleReview = errors.New("no pending reschedule request")
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
	ProcessRecords        []ProcessRecord     `json:"process_records,omitempty"`
}

type CandidateResponseRequest struct {
	Action           string
	ProposedStartsAt *time.Time
	ProposedEndsAt   *time.Time
	Note             string
}

type ReviewRescheduleRequest struct {
	Decision    string
	ProcessedBy string
	Note        string
}

type CandidateResponseResult struct {
	Interview             Interview           `json:"interview"`
	RescheduleRequest     *RescheduleRequest  `json:"reschedule_request,omitempty"`
	Notifications         []NotificationEvent `json:"notifications"`
	NotificationsEnqueued int                 `json:"notifications_enqueued"`
	ProcessRecords        []ProcessRecord     `json:"process_records"`
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
		CandidateState: CandidateResponseAwaiting,
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
	s.repo.AddProcessRecord(ProcessRecord{
		InterviewID: created.ID,
		Action:      "interview.scheduled",
		ActorType:   actorTypeForUser(created.CreatedBy),
		ActorID:     created.CreatedBy,
		Note:        created.Note,
	})
	records := s.repo.ListProcessRecords(created.ID)
	return OperationResult{
		Interview:             created,
		Notifications:         events,
		NotificationsEnqueued: len(events),
		ProcessRecords:        records,
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
	if timeChanged {
		item.CandidateState = CandidateResponseAwaiting
	}

	item.ColorTag = ComputeColorTag(item.Round, item.Status)
	if conflicts := s.detectConflicts(item, item.ID); len(conflicts) > 0 {
		return OperationResult{}, &ConflictError{Conflicts: conflicts}
	}

	updated := s.repo.UpdateInterview(item)
	events := s.repo.EnqueueNotifications(buildNotificationEvents(updated, "interview.updated"))
	s.repo.AddProcessRecord(ProcessRecord{
		InterviewID: updated.ID,
		Action:      "interview.updated",
		ActorType:   actorTypeForUser(updated.CreatedBy),
		ActorID:     updated.CreatedBy,
		Note:        updated.Note,
	})
	records := s.repo.ListProcessRecords(updated.ID)
	return OperationResult{
		Interview:             updated,
		Notifications:         events,
		NotificationsEnqueued: len(events),
		ProcessRecords:        records,
	}, nil
}

func (s *Service) SubmitCandidateResponse(token string, req CandidateResponseRequest) (CandidateResponseResult, error) {
	token = strings.TrimSpace(token)
	item, ok := s.repo.GetInterviewByCandidateToken(token)
	if !ok {
		return CandidateResponseResult{}, fmt.Errorf("%w: %s", ErrCandidateTokenNotFound, token)
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "confirm":
		item.CandidateState = CandidateResponseConfirmed
		if item.Status == StatusReschedulePending {
			item.Status = StatusScheduled
		}
		item.ColorTag = ComputeColorTag(item.Round, item.Status)
		updated := s.repo.UpdateInterview(item)
		events := s.repo.EnqueueNotifications(buildHRNotificationEvents(updated, "interview.candidate_confirmed"))
		s.repo.AddProcessRecord(ProcessRecord{
			InterviewID: updated.ID,
			Action:      "candidate.confirmed",
			ActorType:   "candidate",
			ActorID:     updated.CandidateID,
			Note:        strings.TrimSpace(req.Note),
		})
		records := s.repo.ListProcessRecords(updated.ID)
		return CandidateResponseResult{
			Interview:             updated,
			Notifications:         events,
			NotificationsEnqueued: len(events),
			ProcessRecords:        records,
		}, nil
	case "reschedule":
		if req.ProposedStartsAt == nil || req.ProposedEndsAt == nil {
			return CandidateResponseResult{}, fmt.Errorf("proposed_starts_at and proposed_ends_at are required")
		}
		proposedStart := req.ProposedStartsAt.UTC()
		proposedEnd := req.ProposedEndsAt.UTC()
		if !proposedStart.Before(proposedEnd) {
			return CandidateResponseResult{}, fmt.Errorf("proposed_starts_at must be before proposed_ends_at")
		}

		existingRequest, ok := s.repo.GetRescheduleRequest(item.ID)
		if !ok {
			existingRequest = RescheduleRequest{}
		}
		request := RescheduleRequest{
			ID:               existingRequest.ID,
			InterviewID:      item.ID,
			CandidateID:      item.CandidateID,
			ProposedStartsAt: proposedStart,
			ProposedEndsAt:   proposedEnd,
			Note:             strings.TrimSpace(req.Note),
			Status:           RescheduleRequestPending,
		}
		request = s.repo.SaveRescheduleRequest(request)

		item.Status = StatusReschedulePending
		item.CandidateState = CandidateResponseReschedulePending
		item.ColorTag = ComputeColorTag(item.Round, item.Status)
		updated := s.repo.UpdateInterview(item)

		events := s.repo.EnqueueNotifications(buildHRNotificationEvents(updated, "interview.reschedule_requested"))
		s.repo.AddProcessRecord(ProcessRecord{
			InterviewID: updated.ID,
			Action:      "candidate.reschedule_requested",
			ActorType:   "candidate",
			ActorID:     updated.CandidateID,
			Note:        request.Note,
		})
		records := s.repo.ListProcessRecords(updated.ID)
		return CandidateResponseResult{
			Interview:             updated,
			RescheduleRequest:     &request,
			Notifications:         events,
			NotificationsEnqueued: len(events),
			ProcessRecords:        records,
		}, nil
	default:
		return CandidateResponseResult{}, fmt.Errorf("invalid action: %q", req.Action)
	}
}

func (s *Service) ReviewReschedule(interviewID string, req ReviewRescheduleRequest) (CandidateResponseResult, error) {
	interviewID = strings.TrimSpace(interviewID)
	item, ok := s.repo.GetInterview(interviewID)
	if !ok {
		return CandidateResponseResult{}, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}

	request, ok := s.repo.GetRescheduleRequest(interviewID)
	if !ok || request.Status != RescheduleRequestPending {
		return CandidateResponseResult{}, fmt.Errorf("%w: %s", ErrNoPendingRescheduleReview, interviewID)
	}

	decision := strings.ToLower(strings.TrimSpace(req.Decision))
	processedBy := strings.TrimSpace(req.ProcessedBy)
	note := strings.TrimSpace(req.Note)

	eventType := ""
	action := ""
	switch decision {
	case "accept":
		if !request.ProposedStartsAt.Before(request.ProposedEndsAt) {
			return CandidateResponseResult{}, fmt.Errorf("invalid proposed schedule range")
		}
		item.StartsAt = request.ProposedStartsAt.UTC()
		item.EndsAt = request.ProposedEndsAt.UTC()
		item.Status = StatusRescheduled
		item.CandidateState = CandidateResponseRescheduleAccepted
		item.ColorTag = ComputeColorTag(item.Round, item.Status)
		if conflicts := s.detectConflicts(item, item.ID); len(conflicts) > 0 {
			return CandidateResponseResult{}, &ConflictError{Conflicts: conflicts}
		}
		request.Status = RescheduleRequestAccepted
		request.ProcessedBy = processedBy
		request.ProcessedNote = note
		eventType = "interview.reschedule_accepted"
		action = "hr.reschedule_accepted"
	case "reject":
		item.Status = StatusScheduled
		item.CandidateState = CandidateResponseRescheduleRejected
		item.ColorTag = ComputeColorTag(item.Round, item.Status)
		request.Status = RescheduleRequestRejected
		request.ProcessedBy = processedBy
		request.ProcessedNote = note
		eventType = "interview.reschedule_rejected"
		action = "hr.reschedule_rejected"
	default:
		return CandidateResponseResult{}, fmt.Errorf("invalid decision: %q", req.Decision)
	}

	updated := s.repo.UpdateInterview(item)
	request = s.repo.SaveRescheduleRequest(request)
	events := s.repo.EnqueueNotifications(buildCandidateNotificationEvents(updated, eventType))
	s.repo.AddProcessRecord(ProcessRecord{
		InterviewID: updated.ID,
		Action:      action,
		ActorType:   actorTypeForUser(processedBy),
		ActorID:     processedBy,
		Note:        note,
	})
	records := s.repo.ListProcessRecords(updated.ID)

	return CandidateResponseResult{
		Interview:             updated,
		RescheduleRequest:     &request,
		Notifications:         events,
		NotificationsEnqueued: len(events),
		ProcessRecords:        records,
	}, nil
}

func (s *Service) ProcessRecords(interviewID string) ([]ProcessRecord, error) {
	interviewID = strings.TrimSpace(interviewID)
	if interviewID == "" {
		return nil, fmt.Errorf("interview id is required")
	}
	if _, ok := s.repo.GetInterview(interviewID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}
	return s.repo.ListProcessRecords(interviewID), nil
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

func IsCandidateTokenNotFound(err error) bool {
	return errors.Is(err, ErrCandidateTokenNotFound)
}

func IsNoPendingRescheduleReview(err error) bool {
	return errors.Is(err, ErrNoPendingRescheduleReview)
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

func buildCandidateNotificationEvents(item Interview, eventType string) []NotificationEvent {
	channels := []string{"in_app", "email"}
	events := make([]NotificationEvent, 0, len(channels))
	for _, channel := range channels {
		events = append(events, NotificationEvent{
			InterviewID:   item.ID,
			RecipientID:   item.CandidateID,
			RecipientType: "candidate",
			Channel:       channel,
			EventType:     eventType,
		})
	}
	return events
}

func buildHRNotificationEvents(item Interview, eventType string) []NotificationEvent {
	channels := []string{"in_app", "email"}
	recipient := strings.TrimSpace(item.CreatedBy)
	if recipient == "" {
		recipient = "hr_pool"
	}
	events := make([]NotificationEvent, 0, len(channels))
	for _, channel := range channels {
		events = append(events, NotificationEvent{
			InterviewID:   item.ID,
			RecipientID:   recipient,
			RecipientType: "hr",
			Channel:       channel,
			EventType:     eventType,
		})
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

func actorTypeForUser(actorID string) string {
	if strings.TrimSpace(actorID) == "" {
		return "system"
	}
	return "hr"
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
