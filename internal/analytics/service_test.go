package analytics

import (
	"strings"
	"testing"
	"time"

	"github.com/zplzpl/resume_system/internal/interview"
	"github.com/zplzpl/resume_system/internal/resume"
)

func TestBuildDashboard(t *testing.T) {
	svc := NewService()
	base := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	candidates := []resume.CandidateProfile{
		{ID: "cand_1", StatusLayer: resume.CandidateStatusNew},
		{ID: "cand_2", StatusLayer: resume.CandidateStatusNew},
		{ID: "cand_3", StatusLayer: resume.CandidateStatusScreening},
		{ID: "cand_4", StatusLayer: resume.CandidateStatusInterview},
		{ID: "cand_5", StatusLayer: resume.CandidateStatusOffer},
		{ID: "cand_6", StatusLayer: resume.CandidateStatusHired},
		{ID: "cand_7", StatusLayer: resume.CandidateStatusArchived},
	}

	interviews := []interview.Interview{
		{ID: "int_1", EndsAt: base.Add(1 * time.Hour)},
		{ID: "int_2", EndsAt: base.Add(2 * time.Hour)},
	}

	evaluations := []interview.Evaluation{
		{InterviewID: "int_1", InterviewerID: "iv_a", SubmittedAt: base.Add(3 * time.Hour)},
		{InterviewID: "int_2", InterviewerID: "iv_a", SubmittedAt: base.Add(5 * time.Hour)},
		{InterviewID: "int_2", InterviewerID: "iv_b", SubmittedAt: base.Add(4 * time.Hour)},
		{InterviewID: "int_2", InterviewerID: "iv_b", SubmittedAt: base.Add(90 * time.Minute)}, // clamped to 0
		{InterviewID: "missing", InterviewerID: "iv_c", SubmittedAt: base.Add(4 * time.Hour)},
	}

	dashboard := svc.BuildDashboard(candidates, interviews, evaluations)

	if dashboard.Funnel.TotalCandidates != 6 {
		t.Fatalf("expected total candidates 6, got %d", dashboard.Funnel.TotalCandidates)
	}
	if len(dashboard.Funnel.Stages) != 5 {
		t.Fatalf("expected 5 funnel stages, got %d", len(dashboard.Funnel.Stages))
	}

	if got := dashboard.Funnel.Stages[0]; got.Stage != "new" || got.CandidateCount != 2 || got.ConversionRate != 1 {
		t.Fatalf("unexpected stage[0]: %+v", got)
	}
	if got := dashboard.Funnel.Stages[1]; got.Stage != "screening" || got.CandidateCount != 1 || got.ConversionRate != 0.5 {
		t.Fatalf("unexpected stage[1]: %+v", got)
	}

	if dashboard.Efficiency.TotalFeedbackCount != 4 {
		t.Fatalf("expected total feedback count 4, got %d", dashboard.Efficiency.TotalFeedbackCount)
	}
	if dashboard.Efficiency.AvgFeedbackDurationHours != 1.75 {
		t.Fatalf("expected avg feedback duration 1.75, got %.2f", dashboard.Efficiency.AvgFeedbackDurationHours)
	}
	if len(dashboard.Efficiency.InterviewerWorkload) != 2 {
		t.Fatalf("expected 2 interviewer workloads, got %d", len(dashboard.Efficiency.InterviewerWorkload))
	}
	if got := dashboard.Efficiency.InterviewerWorkload[0]; got.InterviewerID != "iv_a" || got.FeedbackCount != 2 || got.AvgFeedbackDurationHours != 2.5 {
		t.Fatalf("unexpected interviewer workload[0]: %+v", got)
	}
	if got := dashboard.Efficiency.InterviewerWorkload[1]; got.InterviewerID != "iv_b" || got.FeedbackCount != 2 || got.AvgFeedbackDurationHours != 1 {
		t.Fatalf("unexpected interviewer workload[1]: %+v", got)
	}

	if len(dashboard.MetricDefinitions) < 4 {
		t.Fatalf("expected metric definitions, got %d", len(dashboard.MetricDefinitions))
	}
}

func TestBuildDashboardCSV(t *testing.T) {
	svc := NewService()
	dashboard := Dashboard{
		Funnel: FunnelMetrics{
			TotalCandidates: 3,
			Stages: []StageMetric{
				{Stage: "new", CandidateCount: 3, ConversionRate: 1},
				{Stage: "screening", CandidateCount: 2, ConversionRate: 0.67},
			},
		},
		Efficiency: EfficiencyMetrics{
			TotalFeedbackCount:       2,
			AvgFeedbackDurationHours: 1.5,
			InterviewerWorkload: []InterviewerWorkload{
				{InterviewerID: "iv_1", FeedbackCount: 2, AvgFeedbackDurationHours: 1.5},
			},
		},
		MetricDefinitions: []MetricDefinition{
			{
				MetricID:    "stage_conversion_rate",
				Definition:  "test",
				Formula:     "a/b",
				Unit:        "ratio",
				Aggregation: "by_stage",
			},
		},
	}

	content, err := svc.BuildDashboardCSV(dashboard)
	if err != nil {
		t.Fatalf("build csv: %v", err)
	}

	text := string(content)
	if !strings.Contains(text, "section,metric,value,note") {
		t.Fatalf("missing csv header, got %s", text)
	}
	if !strings.Contains(text, "funnel,total_candidates,3") {
		t.Fatalf("missing funnel total row, got %s", text)
	}
	if !strings.Contains(text, "efficiency,total_feedback_count,2") {
		t.Fatalf("missing efficiency total row, got %s", text)
	}
	if !strings.Contains(text, "definition,stage_conversion_rate") {
		t.Fatalf("missing definition row, got %s", text)
	}
}
