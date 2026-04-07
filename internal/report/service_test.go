package report

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func fixedClock() time.Time {
	return time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
}

func sampleRequest() GenerateRequest {
	return GenerateRequest{
		Candidate: CandidateInfo{
			ID:       "cand-1001",
			Name:     "Jane Doe",
			Email:    "jane@example.com",
			Phone:    "+1-555-0100",
			Position: "Backend Engineer",
		},
		Evaluations: []InterviewEvaluation{
			{
				InterviewID:     "round-2",
				InterviewerID:   "u-tech-1",
				InterviewerName: "Tech Lead",
				OverallScore:    4.4,
				Summary:         "Solid architecture thinking.",
				Dimensions: []ScoreDimension{
					{Name: "System Design", Score: 4.5, Comment: "Design choices are realistic."},
					{Name: "Coding", Score: 4.3, Comment: "Clean and maintainable."},
				},
			},
			{
				InterviewID:     "round-1",
				InterviewerID:   "u-hr-1",
				InterviewerName: "HR",
				OverallScore:    4.0,
				Summary:         "Communication is clear and proactive.",
				Dimensions: []ScoreDimension{
					{Name: "Communication", Score: 4.2, Comment: "Clear expression."},
					{Name: "Culture Fit", Score: 3.8, Comment: "Values align with team."},
				},
			},
		},
		GeneratedBy: "hr-user-1",
	}
}

func TestGenerateContainsRequiredSections(t *testing.T) {
	svc := NewService(fixedClock)
	report, err := svc.Generate(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if report.Candidate.Name == "" {
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

func TestGenerateDeterministicIDAndFieldOrder(t *testing.T) {
	svc := NewService(fixedClock)
	req := sampleRequest()

	first, err := svc.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() first error = %v", err)
	}
	second, err := svc.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate() second error = %v", err)
	}

	if first.ReportID != second.ReportID {
		t.Fatalf("expected deterministic report id, got %q vs %q", first.ReportID, second.ReportID)
	}

	if len(first.Scores) < 2 || first.Scores[0].InterviewID > first.Scores[1].InterviewID {
		t.Fatalf("expected scores sorted by interview_id, got %#v", first.Scores)
	}
}

func TestExportJSONMarkdownAndXLSX(t *testing.T) {
	svc := NewService(fixedClock)
	generated, err := svc.Generate(context.Background(), sampleRequest())
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

	_, mdType, mdData, err := svc.Export(generated.ReportID, "markdown")
	if err != nil {
		t.Fatalf("Export(markdown) error = %v", err)
	}
	if !strings.Contains(mdType, "text/markdown") {
		t.Fatalf("unexpected markdown type: %s", mdType)
	}
	if !strings.Contains(string(mdData), "## Interview Scores") {
		t.Fatalf("expected markdown export to include score section")
	}

	_, xlsxType, xlsxData, err := svc.Export(generated.ReportID, "xlsx")
	if err != nil {
		t.Fatalf("Export(xlsx) error = %v", err)
	}
	if !strings.Contains(xlsxType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
		t.Fatalf("unexpected xlsx type: %s", xlsxType)
	}

	reader, err := zip.NewReader(bytes.NewReader(xlsxData), int64(len(xlsxData)))
	if err != nil {
		t.Fatalf("failed to parse xlsx zip: %v", err)
	}

	var sheetXML string
	for _, f := range reader.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			t.Fatalf("failed to open worksheet xml: %v", openErr)
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatalf("failed to read worksheet xml: %v", readErr)
		}
		sheetXML = string(data)
		break
	}
	if sheetXML == "" {
		t.Fatalf("worksheet xml not found in xlsx")
	}
	if !strings.Contains(sheetXML, "candidate_name") {
		t.Fatalf("expected xlsx header row")
	}
	if !strings.Contains(sheetXML, "Jane Doe") {
		t.Fatalf("expected candidate_name in xlsx row")
	}
}

func TestGenerateValidationErrorAndExportErrors(t *testing.T) {
	svc := NewService(fixedClock)

	_, err := svc.Generate(context.Background(), GenerateRequest{})
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

	generated, err := svc.Generate(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	_, _, _, err = svc.Export(generated.ReportID, "pdf")
	if !errors.Is(err, ErrUnsupportedExportFormat) {
		t.Fatalf("expected ErrUnsupportedExportFormat, got %v", err)
	}
}
