package interview

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type TranscriptSpeakerRole string

const (
	TranscriptSpeakerRoleInterviewer TranscriptSpeakerRole = "interviewer"
	TranscriptSpeakerRoleCandidate   TranscriptSpeakerRole = "candidate"
)

type TranscriptSessionStatus string

const (
	TranscriptSessionStatusActive      TranscriptSessionStatus = "active"
	TranscriptSessionStatusInterrupted TranscriptSessionStatus = "interrupted"
	TranscriptSessionStatusEnded       TranscriptSessionStatus = "ended"
)

var (
	ErrTranscriptionSessionNotFound          = errors.New("transcription session not found")
	ErrTranscriptionSessionInterviewMismatch = errors.New("transcription session does not belong to interview")
	ErrTranscriptionSessionInactive          = errors.New("transcription session is not active")
)

type TranscriptSession struct {
	ID           string                  `json:"id"`
	InterviewID  string                  `json:"interview_id"`
	Provider     string                  `json:"provider"`
	Status       TranscriptSessionStatus `json:"status"`
	StartedAt    time.Time               `json:"started_at"`
	UpdatedAt    time.Time               `json:"updated_at"`
	LastSequence int64                   `json:"last_sequence"`
	LastError    string                  `json:"last_error,omitempty"`
}

type TranscriptSegment struct {
	ID          string                `json:"id"`
	InterviewID string                `json:"interview_id"`
	SessionID   string                `json:"session_id"`
	Sequence    int64                 `json:"sequence"`
	SpeakerRole TranscriptSpeakerRole `json:"speaker_role"`
	SpeakerID   string                `json:"speaker_id,omitempty"`
	Text        string                `json:"text"`
	IsFinal     bool                  `json:"is_final"`
	StartedAt   *time.Time            `json:"started_at,omitempty"`
	EndedAt     *time.Time            `json:"ended_at,omitempty"`
	ReceivedAt  time.Time             `json:"received_at"`
}

type StartTranscriptSessionRequest struct {
	Provider string
}

type AppendTranscriptRequest struct {
	SessionID   string
	SpeakerRole string
	SpeakerID   string
	Text        string
	IsFinal     bool
	StartedAt   *time.Time
	EndedAt     *time.Time
}

type StreamTranscriptsRequest struct {
	SessionID     string
	SinceSequence int64
	Limit         int
}

type TranscriptStream struct {
	InterviewID       string              `json:"interview_id"`
	Session           TranscriptSession   `json:"session"`
	Segments          []TranscriptSegment `json:"segments"`
	NextSequence      int64               `json:"next_sequence"`
	HasMore           bool                `json:"has_more"`
	ReconnectRequired bool                `json:"reconnect_required"`
	Message           string              `json:"message,omitempty"`
}

func ParseTranscriptSpeakerRole(raw string) (TranscriptSpeakerRole, error) {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch TranscriptSpeakerRole(raw) {
	case TranscriptSpeakerRoleInterviewer:
		return TranscriptSpeakerRoleInterviewer, nil
	case TranscriptSpeakerRoleCandidate:
		return TranscriptSpeakerRoleCandidate, nil
	default:
		return "", fmt.Errorf("invalid speaker_role: %q", raw)
	}
}
