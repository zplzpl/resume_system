package report

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleGenerateRequest() GenerateRequest {
	return GenerateRequest{
		Candidate: CandidateSnapshot{
			ID:             "cand_000001",
			FullName:       "Alice Zhang",
			Email:          "alice@example.com",
			CurrentTitle:   "Backend Engineer",
			CurrentCompany: "Acme",
			StatusLayer:    "interview",
		},
		Evaluations: []EvaluationSnapshot{
			{
				InterviewID:   "int_000002",
				Round:         "round-2",
				InterviewerID: "iv_2",
				AverageScore:  4.0,
				CapabilityScores: []CapabilityScore{
					{Dimension: "collaboration", Score: 4, Comment: "good teammate"},
					{Dimension: "technical_depth", Score: 4, Comment: "solid"},
				},
				OverallComment: "strong communication",
				Conclusion:     "hire",
				SubmittedAt:    time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC),
			},
			{
				InterviewID:   "int_000001",
				Round:         "round-1",
				InterviewerID: "iv_1",
				AverageScore:  4.5,
				CapabilityScores: []CapabilityScore{
					{Dimension: "problem_solving", Score: 5, Comment: "fast"},
					{Dimension: "communication", Score: 4, Comment: "clear"},
				},
				OverallComment: "strong fundamentals",
				Conclusion:     "strong_hire",
				SubmittedAt:    time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC),
			},
		},
		GeneratedBy: "user_hr",
	}
}

func TestGenerateContainsRequiredSections(t *testing.T) {
	svc := NewService(nil)
	report, err := svc.Generate(sampleGenerateRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if report.Candidate.FullName == "" {
		t.Fatalf("expected candidate info to be present")
	}
	if len(report.Scores) == 0 {
		t.Fatalf("expected scores to be present")
	}
	if report.FinalComment == "" {
		t.Fatalf("expected final comment to be present")
	}
	if report.HiringRecommendation == "" {
		t.Fatalf("expected hiring recommendation to be present")
	}
}

func TestGenerateDeterministicIDAndOrder(t *testing.T) {
	svc := NewService(nil)
	req := sampleGenerateRequest()

	first, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate() first error = %v", err)
	}
	second, err := svc.Generate(req)
	if err != nil {
		t.Fatalf("Generate() second error = %v", err)
	}

	if first.ReportID != second.ReportID {
		t.Fatalf("expected deterministic report id, got %q vs %q", first.ReportID, second.ReportID)
	}
	if !first.GeneratedAt.Equal(second.GeneratedAt) {
		t.Fatalf("expected deterministic generated_at, got %s vs %s", first.GeneratedAt, second.GeneratedAt)
	}
	if len(first.Scores) < 2 || first.Scores[0].Round > first.Scores[1].Round {
		t.Fatalf("expected sorted scores by round, got %#v", first.Scores)
	}
}

func TestExportJSONAndMarkdown(t *testing.T) {
	svc := NewService(nil)
	generated, err := svc.Generate(sampleGenerateRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	_, contentType, jsonData, err := svc.Export(generated.ReportID, "json")
	if err != nil {
		t.Fatalf("Export(json) error = %v", err)
	}
	if contentType != "application/json" {
		t.Fatalf("unexpected content type: %s", contentType)
	}
	if !strings.Contains(string(jsonData), `"hiring_recommendation"`) {
		t.Fatalf("expected json export to include hiring_recommendation")
	}

	_, markdownType, markdownData, err := svc.Export(generated.ReportID, "markdown")
	if err != nil {
		t.Fatalf("Export(markdown) error = %v", err)
	}
	if !strings.Contains(markdownType, "text/markdown") {
		t.Fatalf("unexpected markdown type: %s", markdownType)
	}
	if !strings.Contains(string(markdownData), "## Score Details") {
		t.Fatalf("expected markdown export to include score section")
	}
}

func TestGenerateValidationAndExportErrors(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.Generate(GenerateRequest{})
	var verr ValidationErrors
	if !errors.As(err, &verr) {
		t.Fatalf("expected validation errors, got %v", err)
	}
	if len(verr) == 0 {
		t.Fatalf("expected non-empty validation errors")
	}

	_, _, _, err = svc.Export("missing", "json")
	if !errors.Is(err, ErrReportNotFound) {
		t.Fatalf("expected ErrReportNotFound, got %v", err)
	}

	generated, err := svc.Generate(sampleGenerateRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	_, _, _, err = svc.Export(generated.ReportID, "pdf")
	if !errors.Is(err, ErrUnsupportedExportFormat) {
		t.Fatalf("expected ErrUnsupportedExportFormat, got %v", err)
	}
}
