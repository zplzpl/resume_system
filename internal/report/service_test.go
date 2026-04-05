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
					{Dimension: "communication", Score: 4, Comment: "good teammate"},
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
	if len(report.DimensionComparisons) == 0 {
		t.Fatalf("expected dimension comparisons to be present")
	}
	if len(report.RadarChart.Dimensions) == 0 || len(report.RadarChart.Series) == 0 {
		t.Fatalf("expected radar chart data to be present")
	}
	if report.FinalComment == "" {
		t.Fatalf("expected final comment to be present")
	}
	if report.HiringRecommendation == "" {
		t.Fatalf("expected hiring recommendation to be present")
	}
}

func TestGenerateDimensionComparisonAndRadarConsistency(t *testing.T) {
	svc := NewService(nil)
	report, err := svc.Generate(sampleGenerateRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	var communication DimensionComparison
	found := false
	for _, cmp := range report.DimensionComparisons {
		if cmp.Dimension == "communication" {
			communication = cmp
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected communication comparison to exist")
	}
	if len(communication.Scores) != 2 {
		t.Fatalf("expected communication dimension to include two interviewer scores, got %d", len(communication.Scores))
	}
	if communication.AverageScore != 4.0 || communication.HighestScore != 4.0 || communication.LowestScore != 4.0 {
		t.Fatalf("unexpected communication comparison values: %#v", communication)
	}

	if len(report.RadarChart.Dimensions) != 3 {
		t.Fatalf("expected radar chart to include three dimensions, got %d", len(report.RadarChart.Dimensions))
	}
	if len(report.RadarChart.Series) != len(report.Scores) {
		t.Fatalf("expected one radar series per interviewer, got %d vs %d", len(report.RadarChart.Series), len(report.Scores))
	}

	scoreByInterviewerAndDimension := make(map[string]map[string]float64, len(report.Scores))
	for _, interviewerScore := range report.Scores {
		dims := make(map[string]float64, len(interviewerScore.CapabilityScores))
		for _, dim := range interviewerScore.CapabilityScores {
			dims[dim.Dimension] = float64(dim.Score)
		}
		scoreByInterviewerAndDimension[interviewerScore.InterviewerID] = dims
	}

	for _, radarSeries := range report.RadarChart.Series {
		expectedByDimension, ok := scoreByInterviewerAndDimension[radarSeries.InterviewerID]
		if !ok {
			t.Fatalf("unexpected radar series interviewer id: %s", radarSeries.InterviewerID)
		}
		for _, value := range radarSeries.Values {
			expected, exists := expectedByDimension[value.Dimension]
			if !exists {
				t.Fatalf("radar dimension %q not found in raw interviewer scores", value.Dimension)
			}
			if expected != value.Score {
				t.Fatalf("radar score mismatch for interviewer=%s dimension=%s: got %.2f want %.2f",
					radarSeries.InterviewerID, value.Dimension, value.Score, expected)
			}
		}
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
	if !strings.Contains(string(markdownData), "## Dimension Comparison") {
		t.Fatalf("expected markdown export to include comparison section")
	}
	if !strings.Contains(string(markdownData), "## Radar Chart Data") {
		t.Fatalf("expected markdown export to include radar section")
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
