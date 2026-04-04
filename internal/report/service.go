package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrReportNotFound          = errors.New("report not found")
	ErrUnsupportedExportFormat = errors.New("unsupported export format")
)

type Clock func() time.Time

type Service struct {
	mu      sync.RWMutex
	clock   Clock
	reports map[string]StructuredInterviewReport
}

func NewService(clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		clock:   clock,
		reports: make(map[string]StructuredInterviewReport),
	}
}

func (s *Service) Generate(_ context.Context, req GenerateRequest) (StructuredInterviewReport, error) {
	req = normalizeRequest(req)
	if verr := validateGenerateRequest(req); len(verr) > 0 {
		return StructuredInterviewReport{}, verr
	}

	scores := make([]InterviewerScore, 0, len(req.Evaluations))
	total := 0.0
	for _, eval := range req.Evaluations {
		total += eval.OverallScore
		scores = append(scores, InterviewerScore{
			InterviewID:     eval.InterviewID,
			InterviewerID:   eval.InterviewerID,
			InterviewerName: eval.InterviewerName,
			OverallScore:    eval.OverallScore,
			Dimensions:      eval.Dimensions,
			Summary:         eval.Summary,
		})
	}
	dimensionComparisons := buildDimensionComparisons(scores)
	radarChart := buildRadarChartData(scores, dimensionComparisons)

	averageScore := total / float64(len(req.Evaluations))
	recommendation := recommendHire(averageScore)
	reportID := deterministicReportID(req)
	report := StructuredInterviewReport{
		ReportID:             reportID,
		Candidate:            req.Candidate,
		Scores:               scores,
		DimensionComparisons: dimensionComparisons,
		RadarChart:           radarChart,
		FinalComment:         buildFinalComment(req.Evaluations),
		HiringRecommendation: recommendation,
		AverageScore:         round(averageScore, 2),
		GeneratedBy:          req.GeneratedBy,
		GeneratedAt:          s.clock().UTC(),
	}

	s.mu.Lock()
	s.reports[reportID] = report
	s.mu.Unlock()

	return report, nil
}

func (s *Service) Export(reportID, format string) (fileName string, contentType string, content []byte, err error) {
	s.mu.RLock()
	report, ok := s.reports[reportID]
	s.mu.RUnlock()
	if !ok {
		return "", "", nil, ErrReportNotFound
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		data, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil {
			return "", "", nil, marshalErr
		}
		return fmt.Sprintf("interview-report-%s.json", reportID), "application/json", data, nil
	case "markdown", "md":
		md := buildMarkdown(report)
		return fmt.Sprintf("interview-report-%s.md", reportID), "text/markdown; charset=utf-8", []byte(md), nil
	default:
		return "", "", nil, fmt.Errorf("%w: %s", ErrUnsupportedExportFormat, format)
	}
}

func normalizeRequest(req GenerateRequest) GenerateRequest {
	req.Candidate.ID = strings.TrimSpace(req.Candidate.ID)
	req.Candidate.Name = strings.TrimSpace(req.Candidate.Name)
	req.Candidate.Email = strings.TrimSpace(req.Candidate.Email)
	req.Candidate.Phone = strings.TrimSpace(req.Candidate.Phone)
	req.Candidate.Position = strings.TrimSpace(req.Candidate.Position)
	req.GeneratedBy = strings.TrimSpace(req.GeneratedBy)

	for i := range req.Evaluations {
		req.Evaluations[i].InterviewID = strings.TrimSpace(req.Evaluations[i].InterviewID)
		req.Evaluations[i].InterviewerID = strings.TrimSpace(req.Evaluations[i].InterviewerID)
		req.Evaluations[i].InterviewerName = strings.TrimSpace(req.Evaluations[i].InterviewerName)
		req.Evaluations[i].Summary = strings.TrimSpace(req.Evaluations[i].Summary)
		for j := range req.Evaluations[i].Dimensions {
			req.Evaluations[i].Dimensions[j].Name = strings.TrimSpace(req.Evaluations[i].Dimensions[j].Name)
			req.Evaluations[i].Dimensions[j].Comment = strings.TrimSpace(req.Evaluations[i].Dimensions[j].Comment)
		}

		sort.Slice(req.Evaluations[i].Dimensions, func(a, b int) bool {
			return req.Evaluations[i].Dimensions[a].Name < req.Evaluations[i].Dimensions[b].Name
		})
	}

	sort.Slice(req.Evaluations, func(i, j int) bool {
		left := req.Evaluations[i]
		right := req.Evaluations[j]
		if left.InterviewID != right.InterviewID {
			return left.InterviewID < right.InterviewID
		}
		if left.InterviewerID != right.InterviewerID {
			return left.InterviewerID < right.InterviewerID
		}
		return left.InterviewerName < right.InterviewerName
	})

	return req
}

func validateGenerateRequest(req GenerateRequest) ValidationErrors {
	var errs ValidationErrors
	if req.Candidate.ID == "" {
		errs = append(errs, ValidationError{Field: "candidate.id", Message: "is required"})
	}
	if req.Candidate.Name == "" {
		errs = append(errs, ValidationError{Field: "candidate.name", Message: "is required"})
	}
	if req.Candidate.Position == "" {
		errs = append(errs, ValidationError{Field: "candidate.position", Message: "is required"})
	}
	if len(req.Evaluations) == 0 {
		errs = append(errs, ValidationError{Field: "evaluations", Message: "must contain at least one evaluation"})
	}
	for i, eval := range req.Evaluations {
		prefix := fmt.Sprintf("evaluations[%d]", i)
		if eval.InterviewID == "" {
			errs = append(errs, ValidationError{Field: prefix + ".interview_id", Message: "is required"})
		}
		if eval.InterviewerID == "" {
			errs = append(errs, ValidationError{Field: prefix + ".interviewer_id", Message: "is required"})
		}
		if eval.InterviewerName == "" {
			errs = append(errs, ValidationError{Field: prefix + ".interviewer_name", Message: "is required"})
		}
		if eval.OverallScore < 0 || eval.OverallScore > 5 {
			errs = append(errs, ValidationError{Field: prefix + ".overall_score", Message: "must be between 0 and 5"})
		}
		if len(eval.Dimensions) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".dimensions", Message: "must contain at least one dimension"})
		}
		for j, dim := range eval.Dimensions {
			dp := fmt.Sprintf("%s.dimensions[%d]", prefix, j)
			if dim.Name == "" {
				errs = append(errs, ValidationError{Field: dp + ".name", Message: "is required"})
			}
			if dim.Score < 0 || dim.Score > 5 {
				errs = append(errs, ValidationError{Field: dp + ".score", Message: "must be between 0 and 5"})
			}
		}
	}
	return errs
}

func recommendHire(avg float64) string {
	switch {
	case avg >= 4.2:
		return "Strong Hire"
	case avg >= 3.5:
		return "Hire"
	case avg >= 2.8:
		return "Hold"
	default:
		return "No Hire"
	}
}

func buildFinalComment(evals []InterviewEvaluation) string {
	parts := make([]string, 0, len(evals))
	for _, eval := range evals {
		if eval.Summary == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", eval.InterviewerName, eval.Summary))
	}
	if len(parts) == 0 {
		return "No textual interview comments were provided."
	}
	return strings.Join(parts, " | ")
}

func deterministicReportID(req GenerateRequest) string {
	b, _ := json.Marshal(req)
	hash := sha256.Sum256(b)
	return "rpt_" + hex.EncodeToString(hash[:8])
}

func buildMarkdown(report StructuredInterviewReport) string {
	var b strings.Builder
	b.WriteString("# Interview Report\n\n")
	b.WriteString(fmt.Sprintf("- Report ID: `%s`\n", report.ReportID))
	b.WriteString(fmt.Sprintf("- Generated At: %s\n", report.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Candidate: %s (%s)\n", report.Candidate.Name, report.Candidate.ID))
	b.WriteString(fmt.Sprintf("- Position: %s\n", report.Candidate.Position))
	b.WriteString(fmt.Sprintf("- Average Score: %.2f\n", report.AverageScore))
	b.WriteString(fmt.Sprintf("- Hiring Recommendation: **%s**\n\n", report.HiringRecommendation))

	b.WriteString("## Dimension Comparison\n\n")
	b.WriteString("| Dimension | Average | Highest | Lowest | Interviewer Scores |\n")
	b.WriteString("|---|---:|---:|---:|---|\n")
	for _, cmp := range report.DimensionComparisons {
		parts := make([]string, 0, len(cmp.Scores))
		for _, item := range cmp.Scores {
			parts = append(parts, fmt.Sprintf("%s(%s): %.2f", item.InterviewerName, item.InterviewID, item.Score))
		}
		b.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %.2f | %s |\n",
			cmp.Dimension,
			cmp.AverageScore,
			cmp.HighestScore,
			cmp.LowestScore,
			escapePipe(strings.Join(parts, "; ")),
		))
	}

	b.WriteString("\n## Radar Chart Data\n\n")
	b.WriteString("| Interview | Interviewer | Values |\n")
	b.WriteString("|---|---|---|\n")
	for _, series := range report.RadarChart.Series {
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n",
			series.InterviewID,
			series.InterviewerName,
			escapePipe(formatRadarValues(series.Values)),
		))
	}

	b.WriteString("## Interview Scores\n\n")
	b.WriteString("| Interview | Interviewer | Overall | Dimension | Score | Comment |\n")
	b.WriteString("|---|---|---:|---|---:|---|\n")
	for _, item := range report.Scores {
		for _, dim := range item.Dimensions {
			b.WriteString(fmt.Sprintf("| %s | %s | %.2f | %s | %.2f | %s |\n",
				item.InterviewID,
				item.InterviewerName,
				item.OverallScore,
				dim.Name,
				dim.Score,
				escapePipe(dim.Comment),
			))
		}
	}

	b.WriteString("\n## Final Comment\n\n")
	b.WriteString(report.FinalComment)
	b.WriteString("\n")
	return b.String()
}

func escapePipe(in string) string {
	return strings.ReplaceAll(in, "|", "\\|")
}

func buildDimensionComparisons(scores []InterviewerScore) []DimensionComparison {
	buckets := make(map[string][]DimensionScorePoint)
	for _, interviewerScore := range scores {
		for _, dim := range interviewerScore.Dimensions {
			buckets[dim.Name] = append(buckets[dim.Name], DimensionScorePoint{
				InterviewID:     interviewerScore.InterviewID,
				InterviewerID:   interviewerScore.InterviewerID,
				InterviewerName: interviewerScore.InterviewerName,
				Score:           dim.Score,
			})
		}
	}

	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	sort.Strings(names)

	comparisons := make([]DimensionComparison, 0, len(names))
	for _, name := range names {
		points := buckets[name]
		sort.Slice(points, func(i, j int) bool {
			if points[i].InterviewID != points[j].InterviewID {
				return points[i].InterviewID < points[j].InterviewID
			}
			if points[i].InterviewerID != points[j].InterviewerID {
				return points[i].InterviewerID < points[j].InterviewerID
			}
			return points[i].InterviewerName < points[j].InterviewerName
		})

		total := 0.0
		highest := points[0].Score
		lowest := points[0].Score
		for _, point := range points {
			total += point.Score
			if point.Score > highest {
				highest = point.Score
			}
			if point.Score < lowest {
				lowest = point.Score
			}
		}

		comparisons = append(comparisons, DimensionComparison{
			Dimension:    name,
			AverageScore: round(total/float64(len(points)), 2),
			HighestScore: round(highest, 2),
			LowestScore:  round(lowest, 2),
			Scores:       points,
		})
	}

	return comparisons
}

func buildRadarChartData(scores []InterviewerScore, comparisons []DimensionComparison) RadarChartData {
	dimensions := make([]RadarDimension, 0, len(comparisons))
	for _, comparison := range comparisons {
		dimensions = append(dimensions, RadarDimension{
			Name:     comparison.Dimension,
			MaxScore: 5,
		})
	}

	series := make([]RadarSeries, 0, len(scores))
	for _, interviewerScore := range scores {
		values := make([]RadarDimensionValue, 0, len(interviewerScore.Dimensions))
		for _, dim := range interviewerScore.Dimensions {
			values = append(values, RadarDimensionValue{
				Dimension: dim.Name,
				Score:     round(dim.Score, 2),
			})
		}
		series = append(series, RadarSeries{
			InterviewID:     interviewerScore.InterviewID,
			InterviewerID:   interviewerScore.InterviewerID,
			InterviewerName: interviewerScore.InterviewerName,
			Values:          values,
		})
	}

	return RadarChartData{
		Dimensions: dimensions,
		Series:     series,
	}
}

func formatRadarValues(values []RadarDimensionValue) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%s: %.2f", value.Dimension, value.Score))
	}
	return strings.Join(parts, ", ")
}

func round(v float64, places int) float64 {
	if places <= 0 {
		return float64(int(v + 0.5))
	}
	f := 1.0
	for i := 0; i < places; i++ {
		f *= 10
	}
	return float64(int(v*f+0.5)) / f
}
