package interview

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultNotificationMaxRetries = 2
)

type NotificationTemplate struct {
	ID      string
	Subject string
	Message string
}

type NotificationSender interface {
	Send(event NotificationEvent) (string, error)
}

type NotificationDispatcher struct {
	sender     NotificationSender
	maxRetries int
	now        func() time.Time
}

func NewNotificationDispatcher(sender NotificationSender, maxRetries int) *NotificationDispatcher {
	if sender == nil {
		sender = defaultNotificationSender{}
	}
	if maxRetries < 0 {
		maxRetries = defaultNotificationMaxRetries
	}
	return &NotificationDispatcher{
		sender:     sender,
		maxRetries: maxRetries,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (d *NotificationDispatcher) Dispatch(event NotificationEvent) NotificationEvent {
	if event.MaxRetries < 0 {
		event.MaxRetries = d.maxRetries
	}
	if event.Status == "" {
		event.Status = NotificationStatusPending
	}

	for attempt := 0; attempt <= event.MaxRetries; attempt++ {
		event.RetryCount = attempt
		at := d.now()

		messageID, err := d.sender.Send(event)
		if err == nil {
			event.Status = NotificationStatusSent
			event.LastError = ""
			event.SentAt = cloneTimePtr(&at)
			event.UpdatedAt = at
			event.Attempts = append(event.Attempts, NotificationDeliveryAttempt{
				Attempt:    attempt + 1,
				Status:     NotificationStatusSent,
				MessageID:  messageID,
				OccurredAt: at,
			})
			return event
		}

		event.Status = NotificationStatusFailed
		event.LastError = err.Error()
		event.UpdatedAt = at
		event.Attempts = append(event.Attempts, NotificationDeliveryAttempt{
			Attempt:    attempt + 1,
			Status:     NotificationStatusFailed,
			Error:      err.Error(),
			OccurredAt: at,
		})
	}

	alertAt := d.now()
	event.Status = NotificationStatusAlerted
	event.Alert = &NotificationDeliveryAlert{
		Code:        "DELIVERY_RETRY_EXHAUSTED",
		Message:     "notification delivery failed after max retries",
		TriggeredAt: alertAt,
	}
	event.UpdatedAt = alertAt
	return event
}

func buildNotificationEvents(item Interview, eventType string) []NotificationEvent {
	recipients := make([]notificationRecipient, 0, 1+len(item.InterviewerIDs))
	recipients = append(recipients, notificationRecipient{ID: item.CandidateID, Type: "candidate"})
	for _, interviewerID := range item.InterviewerIDs {
		recipients = append(recipients, notificationRecipient{ID: interviewerID, Type: "interviewer"})
	}

	channels := []NotificationChannel{
		NotificationChannelInApp,
		NotificationChannelEmail,
		NotificationChannelSMS,
	}

	events := make([]NotificationEvent, 0, len(recipients)*len(channels))
	for _, recipient := range recipients {
		for _, channel := range channels {
			template := resolveNotificationTemplate(eventType, recipient.Type, channel)
			templateVariables := buildTemplateVariables(item, eventType, recipient, channel)

			events = append(events, NotificationEvent{
				InterviewID:       item.ID,
				RecipientID:       recipient.ID,
				RecipientType:     recipient.Type,
				Channel:           channel,
				EventType:         eventType,
				TemplateID:        template.ID,
				TemplateVariables: templateVariables,
				Subject:           renderTemplate(template.Subject, templateVariables),
				Message:           renderTemplate(template.Message, templateVariables),
				Status:            NotificationStatusPending,
				RetryCount:        0,
				MaxRetries:        defaultNotificationMaxRetries,
			})
		}
	}

	return events
}

type notificationRecipient struct {
	ID   string
	Type string
}

func buildTemplateVariables(item Interview, eventType string, recipient notificationRecipient, channel NotificationChannel) map[string]string {
	return map[string]string{
		"event_type":     eventType,
		"interview_id":   item.ID,
		"candidate_id":   item.CandidateID,
		"recipient_id":   recipient.ID,
		"recipient_type": recipient.Type,
		"round":          item.Round,
		"status":         string(item.Status),
		"starts_at":      item.StartsAt.Format(time.RFC3339),
		"ends_at":        item.EndsAt.Format(time.RFC3339),
		"channel":        string(channel),
	}
}

func resolveNotificationTemplate(eventType, recipientType string, channel NotificationChannel) NotificationTemplate {
	if tpl, ok := defaultNotificationTemplates[notificationTemplateKey(eventType, recipientType, channel)]; ok {
		return tpl
	}
	if tpl, ok := defaultNotificationTemplates[notificationTemplateKey(eventType, "*", channel)]; ok {
		return tpl
	}
	return NotificationTemplate{
		ID:      fmt.Sprintf("%s.%s.fallback", eventType, channel),
		Subject: "Interview update",
		Message: "Interview {{interview_id}} updated. Starts at {{starts_at}}.",
	}
}

func notificationTemplateKey(eventType, recipientType string, channel NotificationChannel) string {
	return strings.Join([]string{eventType, recipientType, string(channel)}, "|")
}

func renderTemplate(template string, variables map[string]string) string {
	out := template
	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = strings.ReplaceAll(out, "{{"+key+"}}", variables[key])
	}
	return out
}

type defaultNotificationSender struct{}

func (s defaultNotificationSender) Send(event NotificationEvent) (string, error) {
	if shouldFailNotification(event) {
		return "", fmt.Errorf("simulated delivery failure for %s channel", event.Channel)
	}
	messageID := fmt.Sprintf("%s-attempt-%d", event.TraceID, event.RetryCount+1)
	return messageID, nil
}

func shouldFailNotification(event NotificationEvent) bool {
	recipientID := strings.ToLower(strings.TrimSpace(event.RecipientID))
	if strings.Contains(recipientID, "notify_fail") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(event.TemplateVariables["force_fail"]), "true") {
		return true
	}
	return false
}

var defaultNotificationTemplates = map[string]NotificationTemplate{
	notificationTemplateKey("interview.created", "candidate", NotificationChannelInApp): {
		ID:      "interview.created.candidate.in_app",
		Subject: "Interview scheduled",
		Message: "Interview {{interview_id}} is scheduled at {{starts_at}} ({{round}}).",
	},
	notificationTemplateKey("interview.created", "candidate", NotificationChannelEmail): {
		ID:      "interview.created.candidate.email",
		Subject: "Interview confirmation for {{starts_at}}",
		Message: "Your interview {{interview_id}} is scheduled for {{starts_at}} to {{ends_at}}. Round: {{round}}.",
	},
	notificationTemplateKey("interview.created", "candidate", NotificationChannelSMS): {
		ID:      "interview.created.candidate.sms",
		Subject: "Interview reminder",
		Message: "Interview {{interview_id}} starts at {{starts_at}}.",
	},
	notificationTemplateKey("interview.created", "interviewer", NotificationChannelInApp): {
		ID:      "interview.created.interviewer.in_app",
		Subject: "New interview assigned",
		Message: "Interview {{interview_id}} assigned to you at {{starts_at}} (candidate {{candidate_id}}).",
	},
	notificationTemplateKey("interview.created", "interviewer", NotificationChannelEmail): {
		ID:      "interview.created.interviewer.email",
		Subject: "Interview assignment {{interview_id}}",
		Message: "Please interview candidate {{candidate_id}} at {{starts_at}}. Round: {{round}}.",
	},
	notificationTemplateKey("interview.created", "interviewer", NotificationChannelSMS): {
		ID:      "interview.created.interviewer.sms",
		Subject: "Interview assignment",
		Message: "Interview {{interview_id}} with candidate {{candidate_id}} at {{starts_at}}.",
	},
	notificationTemplateKey("interview.updated", "*", NotificationChannelInApp): {
		ID:      "interview.updated.shared.in_app",
		Subject: "Interview updated",
		Message: "Interview {{interview_id}} has updates. New slot: {{starts_at}} to {{ends_at}}.",
	},
	notificationTemplateKey("interview.updated", "*", NotificationChannelEmail): {
		ID:      "interview.updated.shared.email",
		Subject: "Interview update {{interview_id}}",
		Message: "Interview {{interview_id}} has been updated. Start {{starts_at}}, end {{ends_at}}, status {{status}}.",
	},
	notificationTemplateKey("interview.updated", "*", NotificationChannelSMS): {
		ID:      "interview.updated.shared.sms",
		Subject: "Interview updated",
		Message: "Interview {{interview_id}} updated. {{starts_at}} to {{ends_at}}.",
	},
}
