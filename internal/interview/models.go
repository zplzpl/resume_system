package interview

import (
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusScheduled         Status = "scheduled"
	StatusRescheduled       Status = "rescheduled"
	StatusReschedulePending Status = "reschedule_pending"
	StatusCancelled         Status = "cancelled"
	StatusCompleted         Status = "completed"
)

type CandidateResponseStatus string

const (
	CandidateResponseAwaiting           CandidateResponseStatus = "awaiting_candidate"
	CandidateResponseConfirmed          CandidateResponseStatus = "confirmed"
	CandidateResponseReschedulePending  CandidateResponseStatus = "reschedule_pending"
	CandidateResponseRescheduleAccepted CandidateResponseStatus = "reschedule_accepted"
	CandidateResponseRescheduleRejected CandidateResponseStatus = "reschedule_rejected"
)

type RescheduleRequestStatus string

const (
	RescheduleRequestPending  RescheduleRequestStatus = "pending"
	RescheduleRequestAccepted RescheduleRequestStatus = "accepted"
	RescheduleRequestRejected RescheduleRequestStatus = "rejected"
)

type CalendarView string

const (
	CalendarViewDay   CalendarView = "day"
	CalendarViewWeek  CalendarView = "week"
	CalendarViewMonth CalendarView = "month"
)

type Interview struct {
	ID             string                  `json:"id"`
	CandidateID    string                  `json:"candidate_id"`
	InterviewerIDs []string                `json:"interviewer_ids"`
	CandidateToken string                  `json:"candidate_token,omitempty"`
	Round          string                  `json:"round"`
	Status         Status                  `json:"status"`
	CandidateState CandidateResponseStatus `json:"candidate_state"`
	StartsAt       time.Time               `json:"starts_at"`
	EndsAt         time.Time               `json:"ends_at"`
	ColorTag       string                  `json:"color_tag"`
	Note           string                  `json:"note,omitempty"`
	CreatedBy      string                  `json:"created_by,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type Conflict struct {
	Type         string    `json:"type"`
	InterviewID  string    `json:"interview_id"`
	EntityType   string    `json:"entity_type"`
	EntityID     string    `json:"entity_id"`
	ConflictFrom time.Time `json:"conflict_from"`
	ConflictTo   time.Time `json:"conflict_to"`
	Message      string    `json:"message"`
}

type NotificationEvent struct {
	ID            string    `json:"id"`
	InterviewID   string    `json:"interview_id"`
	RecipientID   string    `json:"recipient_id"`
	RecipientType string    `json:"recipient_type"`
	Channel       string    `json:"channel"`
	EventType     string    `json:"event_type"`
	CreatedAt     time.Time `json:"created_at"`
}

type RescheduleRequest struct {
	ID               string                  `json:"id"`
	InterviewID      string                  `json:"interview_id"`
	CandidateID      string                  `json:"candidate_id"`
	ProposedStartsAt time.Time               `json:"proposed_starts_at"`
	ProposedEndsAt   time.Time               `json:"proposed_ends_at"`
	Note             string                  `json:"note,omitempty"`
	Status           RescheduleRequestStatus `json:"status"`
	ProcessedBy      string                  `json:"processed_by,omitempty"`
	ProcessedNote    string                  `json:"processed_note,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at"`
}

type ProcessRecord struct {
	ID          string    `json:"id"`
	InterviewID string    `json:"interview_id"`
	Action      string    `json:"action"`
	ActorType   string    `json:"actor_type"`
	ActorID     string    `json:"actor_id"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type CalendarResult struct {
	View      CalendarView `json:"view"`
	RangeFrom time.Time    `json:"range_from"`
	RangeTo   time.Time    `json:"range_to"`
	Items     []Interview  `json:"items"`
}

func ParseStatus(raw string) (Status, error) {
	switch Status(strings.ToLower(strings.TrimSpace(raw))) {
	case StatusScheduled:
		return StatusScheduled, nil
	case StatusRescheduled:
		return StatusRescheduled, nil
	case StatusReschedulePending:
		return StatusReschedulePending, nil
	case StatusCancelled:
		return StatusCancelled, nil
	case StatusCompleted:
		return StatusCompleted, nil
	default:
		return "", fmt.Errorf("invalid interview status: %q", raw)
	}
}

func ParseCalendarView(raw string) (CalendarView, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return CalendarViewWeek, nil
	}
	switch CalendarView(raw) {
	case CalendarViewDay:
		return CalendarViewDay, nil
	case CalendarViewWeek:
		return CalendarViewWeek, nil
	case CalendarViewMonth:
		return CalendarViewMonth, nil
	default:
		return "", fmt.Errorf("invalid calendar view: %q", raw)
	}
}

func ComputeColorTag(round string, status Status) string {
	if status == StatusCancelled {
		return "status-cancelled"
	}
	if status == StatusReschedulePending {
		return "status-reschedule-pending"
	}
	if status == StatusRescheduled {
		return "status-rescheduled"
	}

	switch strings.ToLower(strings.TrimSpace(round)) {
	case "", "round-1", "first":
		return "round-1"
	case "round-2", "second":
		return "round-2"
	case "round-3", "third":
		return "round-3"
	default:
		return "round-other"
	}
}

func IsActiveStatus(status Status) bool {
	return status == StatusScheduled || status == StatusRescheduled || status == StatusReschedulePending
}
