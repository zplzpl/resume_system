package audit

import "time"

const (
	ActionAuthLogin                 = "auth.login"
	ActionAuthLogout                = "auth.logout"
	ActionResumeDelete              = "resume.delete"
	ActionInterviewEvaluationSubmit = "interview.evaluation.submit"
	ActionInterviewEvaluationModify = "interview.evaluation.modify"
)

type Event struct {
	ID         string            `json:"id"`
	ActionType string            `json:"action_type"`
	OperatorID string            `json:"operator_id,omitempty"`
	ObjectType string            `json:"object_type"`
	ObjectID   string            `json:"object_id,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type RecordInput struct {
	ActionType string
	OperatorID string
	ObjectType string
	ObjectID   string
	Metadata   map[string]string
}

type QueryFilter struct {
	ActionType string
	OperatorID string
	ObjectType string
	ObjectID   string
	From       *time.Time
	To         *time.Time
	Limit      int
}
