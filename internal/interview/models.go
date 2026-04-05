package interview

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Status string

const (
	StatusScheduled   Status = "scheduled"
	StatusRescheduled Status = "rescheduled"
	StatusCancelled   Status = "cancelled"
	StatusCompleted   Status = "completed"
)

type CalendarView string

const (
	CalendarViewDay   CalendarView = "day"
	CalendarViewWeek  CalendarView = "week"
	CalendarViewMonth CalendarView = "month"
)

type Interview struct {
	ID             string    `json:"id"`
	CandidateID    string    `json:"candidate_id"`
	InterviewerIDs []string  `json:"interviewer_ids"`
	Round          string    `json:"round"`
	Status         Status    `json:"status"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	ColorTag       string    `json:"color_tag"`
	Note           string    `json:"note,omitempty"`
	CreatedBy      string    `json:"created_by,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
	ID                string                        `json:"id"`
	InterviewID       string                        `json:"interview_id"`
	RecipientID       string                        `json:"recipient_id"`
	RecipientType     string                        `json:"recipient_type"`
	Channel           NotificationChannel           `json:"channel"`
	EventType         string                        `json:"event_type"`
	TemplateID        string                        `json:"template_id,omitempty"`
	TemplateVariables map[string]string             `json:"template_variables,omitempty"`
	Subject           string                        `json:"subject,omitempty"`
	Message           string                        `json:"message,omitempty"`
	Status            NotificationDeliveryStatus    `json:"status"`
	RetryCount        int                           `json:"retry_count"`
	MaxRetries        int                           `json:"max_retries"`
	LastError         string                        `json:"last_error,omitempty"`
	TraceID           string                        `json:"trace_id,omitempty"`
	Attempts          []NotificationDeliveryAttempt `json:"attempts,omitempty"`
	Alert             *NotificationDeliveryAlert    `json:"alert,omitempty"`
	SentAt            *time.Time                    `json:"sent_at,omitempty"`
	CreatedAt         time.Time                     `json:"created_at"`
	UpdatedAt         time.Time                     `json:"updated_at"`
}

type NotificationChannel string

const (
	NotificationChannelInApp NotificationChannel = "in_app"
	NotificationChannelEmail NotificationChannel = "email"
	NotificationChannelSMS   NotificationChannel = "sms"
)

type NotificationDeliveryStatus string

const (
	NotificationStatusPending NotificationDeliveryStatus = "pending"
	NotificationStatusSent    NotificationDeliveryStatus = "sent"
	NotificationStatusFailed  NotificationDeliveryStatus = "failed"
	NotificationStatusAlerted NotificationDeliveryStatus = "alerted"
)

type NotificationDeliveryAttempt struct {
	Attempt    int                        `json:"attempt"`
	Status     NotificationDeliveryStatus `json:"status"`
	MessageID  string                     `json:"message_id,omitempty"`
	Error      string                     `json:"error,omitempty"`
	OccurredAt time.Time                  `json:"occurred_at"`
}

type NotificationDeliveryAlert struct {
	Code        string    `json:"code"`
	Message     string    `json:"message"`
	TriggeredAt time.Time `json:"triggered_at"`
}

type CalendarResult struct {
	View      CalendarView `json:"view"`
	RangeFrom time.Time    `json:"range_from"`
	RangeTo   time.Time    `json:"range_to"`
	Items     []Interview  `json:"items"`
}

var (
	ErrQuestionRecommendationNotFound = errors.New("question recommendation not found")
)

type QuestionCategory string

const (
	QuestionCategoryExperienceFollowUp   QuestionCategory = "experience_follow_up"
	QuestionCategoryCapabilityAssessment QuestionCategory = "capability_assessment"
)

type CandidateSnapshot struct {
	ID                    string
	FullName              string
	CurrentCompany        string
	CurrentTitle          string
	HighestEducation      string
	TotalExperienceMonths int
	Skills                []string
}

type GenerateQuestionRecommendationRequest struct {
	JobTitle       string
	JobDescription string
}

type RecommendedQuestion struct {
	Category  QuestionCategory `json:"category"`
	Question  string           `json:"question"`
	Focus     string           `json:"focus"`
	Reference string           `json:"reference,omitempty"`
}

type QuestionRecommendation struct {
	InterviewID     string                `json:"interview_id"`
	CandidateID     string                `json:"candidate_id"`
	Round           string                `json:"round"`
	JobTitle        string                `json:"job_title,omitempty"`
	JobDescription  string                `json:"job_description,omitempty"`
	Questions       []RecommendedQuestion `json:"questions"`
	FallbackUsed    bool                  `json:"fallback_used"`
	FallbackReason  string                `json:"fallback_reason,omitempty"`
	GeneratedSource string                `json:"generated_source"`
	GeneratedAt     time.Time             `json:"generated_at"`
}

func ParseStatus(raw string) (Status, error) {
	switch Status(strings.ToLower(strings.TrimSpace(raw))) {
	case StatusScheduled:
		return StatusScheduled, nil
	case StatusRescheduled:
		return StatusRescheduled, nil
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
	return status == StatusScheduled || status == StatusRescheduled
}
