package interview

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultTranscriptStreamLimit = 50
	maxTranscriptStreamLimit     = 200
)

func (s *Service) StartTranscriptSession(interviewID string, req StartTranscriptSessionRequest) (TranscriptSession, error) {
	interviewID = strings.TrimSpace(interviewID)
	if err := s.ensureInterviewExists(interviewID); err != nil {
		return TranscriptSession{}, err
	}

	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		provider = "speech_recognition"
	}

	item := TranscriptSession{
		InterviewID: interviewID,
		Provider:    provider,
		Status:      TranscriptSessionStatusActive,
	}
	return s.repo.CreateTranscriptSession(item), nil
}

func (s *Service) AppendTranscript(interviewID string, req AppendTranscriptRequest) (TranscriptSegment, TranscriptSession, error) {
	interviewID = strings.TrimSpace(interviewID)
	if err := s.ensureInterviewExists(interviewID); err != nil {
		return TranscriptSegment{}, TranscriptSession{}, err
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return TranscriptSegment{}, TranscriptSession{}, fmt.Errorf("session_id is required")
	}
	session, err := s.getTranscriptSessionForInterview(interviewID, sessionID)
	if err != nil {
		return TranscriptSegment{}, TranscriptSession{}, err
	}
	if session.Status != TranscriptSessionStatusActive {
		message := strings.TrimSpace(session.LastError)
		if message == "" {
			message = "session interrupted"
		}
		return TranscriptSegment{}, TranscriptSession{}, fmt.Errorf("%w: %s", ErrTranscriptionSessionInactive, message)
	}

	speakerRole, err := ParseTranscriptSpeakerRole(req.SpeakerRole)
	if err != nil {
		return TranscriptSegment{}, TranscriptSession{}, err
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		return TranscriptSegment{}, TranscriptSession{}, fmt.Errorf("text is required")
	}

	startedAt := cloneTimePtr(req.StartedAt)
	endedAt := cloneTimePtr(req.EndedAt)
	if startedAt != nil && endedAt != nil && endedAt.Before(*startedAt) {
		return TranscriptSegment{}, TranscriptSession{}, fmt.Errorf("ended_at must be after started_at")
	}

	stored := s.repo.AppendTranscriptSegment(TranscriptSegment{
		InterviewID: interviewID,
		SessionID:   session.ID,
		SpeakerRole: speakerRole,
		SpeakerID:   strings.TrimSpace(req.SpeakerID),
		Text:        text,
		IsFinal:     req.IsFinal,
		StartedAt:   startedAt,
		EndedAt:     endedAt,
	})

	session.LastSequence = stored.Sequence
	session.LastError = ""
	session.UpdatedAt = time.Now().UTC()
	session = s.repo.UpdateTranscriptSession(session)

	return stored, session, nil
}

func (s *Service) StreamTranscripts(interviewID string, req StreamTranscriptsRequest) (TranscriptStream, error) {
	interviewID = strings.TrimSpace(interviewID)
	if err := s.ensureInterviewExists(interviewID); err != nil {
		return TranscriptStream{}, err
	}

	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return TranscriptStream{}, fmt.Errorf("session_id is required")
	}

	session, err := s.getTranscriptSessionForInterview(interviewID, sessionID)
	if err != nil {
		return TranscriptStream{}, err
	}

	sinceSequence := req.SinceSequence
	if sinceSequence < 0 {
		sinceSequence = 0
	}

	limit := req.Limit
	if limit <= 0 {
		limit = defaultTranscriptStreamLimit
	}
	if limit > maxTranscriptStreamLimit {
		limit = maxTranscriptStreamLimit
	}

	segments, hasMore := s.repo.ListTranscriptSegmentsByInterviewSince(interviewID, sinceSequence, limit)
	nextSequence := sinceSequence
	if len(segments) > 0 {
		nextSequence = segments[len(segments)-1].Sequence
	} else if session.LastSequence > nextSequence {
		nextSequence = session.LastSequence
	}

	stream := TranscriptStream{
		InterviewID:  interviewID,
		Session:      session,
		Segments:     segments,
		NextSequence: nextSequence,
		HasMore:      hasMore,
	}

	if session.Status == TranscriptSessionStatusInterrupted {
		stream.ReconnectRequired = true
		stream.Message = session.LastError
		if stream.Message == "" {
			stream.Message = "transcription interrupted, reconnect required"
		}
	}

	if session.Status == TranscriptSessionStatusEnded {
		stream.Message = "transcription session ended"
	}

	return stream, nil
}

func (s *Service) MarkTranscriptSessionInterrupted(interviewID, sessionID, reason string) (TranscriptSession, error) {
	interviewID = strings.TrimSpace(interviewID)
	if err := s.ensureInterviewExists(interviewID); err != nil {
		return TranscriptSession{}, err
	}

	session, err := s.getTranscriptSessionForInterview(interviewID, sessionID)
	if err != nil {
		return TranscriptSession{}, err
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "speech recognition stream interrupted"
	}

	session.Status = TranscriptSessionStatusInterrupted
	session.LastError = reason
	session.UpdatedAt = time.Now().UTC()

	return s.repo.UpdateTranscriptSession(session), nil
}

func (s *Service) ReconnectTranscriptSession(interviewID, sessionID string) (TranscriptSession, error) {
	interviewID = strings.TrimSpace(interviewID)
	if err := s.ensureInterviewExists(interviewID); err != nil {
		return TranscriptSession{}, err
	}

	session, err := s.getTranscriptSessionForInterview(interviewID, sessionID)
	if err != nil {
		return TranscriptSession{}, err
	}

	session.Status = TranscriptSessionStatusActive
	session.LastError = ""
	session.UpdatedAt = time.Now().UTC()

	return s.repo.UpdateTranscriptSession(session), nil
}

func (s *Service) getTranscriptSessionForInterview(interviewID, sessionID string) (TranscriptSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return TranscriptSession{}, fmt.Errorf("session_id is required")
	}
	session, ok := s.repo.GetTranscriptSession(sessionID)
	if !ok {
		return TranscriptSession{}, fmt.Errorf("%w: %s", ErrTranscriptionSessionNotFound, sessionID)
	}
	if session.InterviewID != interviewID {
		return TranscriptSession{}, fmt.Errorf("%w: session=%s interview=%s", ErrTranscriptionSessionInterviewMismatch, sessionID, interviewID)
	}
	return session, nil
}

func (s *Service) ensureInterviewExists(interviewID string) error {
	if _, ok := s.repo.GetInterview(interviewID); !ok {
		return fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}
	return nil
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := in.UTC()
	return &out
}
