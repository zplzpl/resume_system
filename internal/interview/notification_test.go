package interview

import (
	"strings"
	"testing"
	"time"
)

func TestBuildNotificationEventsUsesTemplateVariables(t *testing.T) {
	start := time.Date(2026, 4, 8, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	item := Interview{
		ID:             "int_000111",
		CandidateID:    "cand_001",
		InterviewerIDs: []string{"iv_001"},
		Round:          "round-2",
		Status:         StatusScheduled,
		StartsAt:       start,
		EndsAt:         end,
	}

	events := buildNotificationEvents(item, "interview.created")
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d", len(events))
	}

	candidateEmail := findNotificationEvent(events, "cand_001", NotificationChannelEmail)
	if candidateEmail.TemplateID != "interview.created.candidate.email" {
		t.Fatalf("unexpected template id: %q", candidateEmail.TemplateID)
	}
	if candidateEmail.Status != NotificationStatusPending {
		t.Fatalf("expected pending status, got %q", candidateEmail.Status)
	}
	if candidateEmail.MaxRetries != defaultNotificationMaxRetries {
		t.Fatalf("expected max retries %d, got %d", defaultNotificationMaxRetries, candidateEmail.MaxRetries)
	}
	if strings.Contains(candidateEmail.Subject, "{{") || strings.Contains(candidateEmail.Message, "{{") {
		t.Fatalf("expected rendered template without placeholders, subject=%q message=%q", candidateEmail.Subject, candidateEmail.Message)
	}
	if !strings.Contains(candidateEmail.Message, item.ID) {
		t.Fatalf("expected message to include interview id, got %q", candidateEmail.Message)
	}
	if !strings.Contains(candidateEmail.Message, start.Format(time.RFC3339)) {
		t.Fatalf("expected message to include starts_at, got %q", candidateEmail.Message)
	}
}

func TestServiceCreateDispatchesRetryAndAlert(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(repo)

	start := time.Date(2026, 4, 9, 9, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	result, err := svc.Create(CreateRequest{
		CandidateID:    "notify_fail_candidate",
		InterviewerIDs: []string{"iv_ok"},
		StartsAt:       start,
		EndsAt:         end,
		Round:          "round-1",
		CreatedBy:      "user_hr",
	})
	if err != nil {
		t.Fatalf("create interview: %v", err)
	}

	if result.NotificationsEnqueued != 6 {
		t.Fatalf("expected 6 notifications enqueued, got %d", result.NotificationsEnqueued)
	}

	alertedCount := 0
	sentCount := 0
	for _, event := range result.Notifications {
		if event.TraceID == "" {
			t.Fatalf("expected trace id for event %s", event.ID)
		}
		switch event.RecipientID {
		case "notify_fail_candidate":
			if event.Status != NotificationStatusAlerted {
				t.Fatalf("expected alerted status for failing recipient, got %q", event.Status)
			}
			if len(event.Attempts) != defaultNotificationMaxRetries+1 {
				t.Fatalf("expected %d attempts, got %d", defaultNotificationMaxRetries+1, len(event.Attempts))
			}
			if event.Alert == nil {
				t.Fatalf("expected alert for exhausted retries")
			}
			if event.SentAt != nil {
				t.Fatalf("expected sent_at nil for failed delivery")
			}
			alertedCount++
		case "iv_ok":
			if event.Status != NotificationStatusSent {
				t.Fatalf("expected sent status for healthy recipient, got %q", event.Status)
			}
			if len(event.Attempts) != 1 {
				t.Fatalf("expected 1 attempt for successful delivery, got %d", len(event.Attempts))
			}
			if event.Alert != nil {
				t.Fatalf("expected no alert for successful delivery")
			}
			if event.SentAt == nil {
				t.Fatalf("expected sent_at for successful delivery")
			}
			sentCount++
		default:
			t.Fatalf("unexpected recipient id: %s", event.RecipientID)
		}
	}

	if alertedCount != 3 {
		t.Fatalf("expected 3 alerted events for failing candidate across channels, got %d", alertedCount)
	}
	if sentCount != 3 {
		t.Fatalf("expected 3 successful events for interviewer across channels, got %d", sentCount)
	}
}

func findNotificationEvent(events []NotificationEvent, recipientID string, channel NotificationChannel) NotificationEvent {
	for _, event := range events {
		if event.RecipientID == recipientID && event.Channel == channel {
			return event
		}
	}
	return NotificationEvent{}
}
