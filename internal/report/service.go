package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

func (s *Service) Generate(req GenerateRequest) (StructuredInterviewReport, error) {
	req = normalizeRequest(req)
	if verr := validateGenerateRequest(req); len(verr) > 0 {
		return StructuredInterviewReport{}, verr
	}

	scores := make([]EvaluationScore, 0, len(req.Evaluations))
	conclusionBreakdown := make(map[string]int)
	total := 0.0
	finalComments := make([]string, 0, len(req.Evaluations))
	generatedAt := time.Time{}

	for _, item := range req.Evaluations {
		total += item.AverageScore
		conclusionBreakdown[item.Conclusion]++
		scores = append(scores, EvaluationScore{
			InterviewID:      item.InterviewID,
			Round:            item.Round,
			InterviewerID:    item.InterviewerID,
			AverageScore:     item.AverageScore,
			CapabilityScores: append([]CapabilityScore(nil), item.CapabilityScores...),
			OverallComment:   item.OverallComment,
			Conclusion:       item.Conclusion,
			SubmittedAt:      item.SubmittedAt,
		})
		if item.OverallComment != "" {
			finalComments = append(finalComments, fmt.Sprintf("%s/%s: %s", item.Round, item.InterviewerID, item.OverallComment))
		}
		if item.SubmittedAt.After(generatedAt) {
			generatedAt = item.SubmittedAt
		}
	}

	if generatedAt.IsZero() {
		generatedAt = s.clock().UTC()
	} else {
		generatedAt = generatedAt.UTC()
	}

	average := round(total/float64(len(scores)), 2)
	reportID := deterministicReportID(req)
	report := StructuredInterviewReport{
		ReportID:              reportID,
		Candidate:             req.Candidate,
		Scores:                scores,
		FinalComment:          buildFinalComment(finalComments),
		HiringRecommendation:  recommendHire(average),
		AverageScore:          average,
		ConclusionBreakdown:   conclusionBreakdown,
		SourceEvaluationCount: len(scores),
		GeneratedBy:           req.GeneratedBy,
		GeneratedAt:           generatedAt,
	}

	s.mu.Lock()
	s.reports[reportID] = cloneReport(report)
	s.mu.Unlock()
	return report, nil
}

func (s *Service) Export(reportID, format string) (fileName string, contentType string, content []byte, err error) {
	s.mu.RLock()
	report, ok := s.reports[strings.TrimSpace(reportID)]
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
		return fmt.Sprintf("interview-report-%s.json", report.ReportID), "application/json", data, nil
	case "markdown", "md":
		return fmt.Sprintf("interview-report-%s.md", report.ReportID), "text/markdown; charset=utf-8", []byte(buildMarkdown(report)), nil
	default:
		return "", "", nil, fmt.Errorf("%w: %s", ErrUnsupportedExportFormat, format)
	}
}

func normalizeRequest(req GenerateRequest) GenerateRequest {
	req.Candidate.ID = strings.TrimSpace(req.Candidate.ID)
	req.Candidate.FullName = strings.TrimSpace(req.Candidate.FullName)
	req.Candidate.Email = strings.TrimSpace(req.Candidate.Email)
	req.Candidate.Phone = strings.TrimSpace(req.Candidate.Phone)
	req.Candidate.CurrentTitle = strings.TrimSpace(req.Candidate.CurrentTitle)
	req.Candidate.CurrentCompany = strings.TrimSpace(req.Candidate.CurrentCompany)
	req.Candidate.StatusLayer = strings.TrimSpace(req.Candidate.StatusLayer)
	req.GeneratedBy = strings.TrimSpace(req.GeneratedBy)

	for i := range req.Evaluations {
		req.Evaluations[i].InterviewID = strings.TrimSpace(req.Evaluations[i].InterviewID)
		req.Evaluations[i].Round = strings.TrimSpace(req.Evaluations[i].Round)
		req.Evaluations[i].InterviewerID = strings.TrimSpace(req.Evaluations[i].InterviewerID)
		req.Evaluations[i].OverallComment = strings.TrimSpace(req.Evaluations[i].OverallComment)
		req.Evaluations[i].Conclusion = strings.ToLower(strings.TrimSpace(req.Evaluations[i].Conclusion))
		req.Evaluations[i].SubmittedAt = req.Evaluations[i].SubmittedAt.UTC()
		for j := range req.Evaluations[i].CapabilityScores {
			req.Evaluations[i].CapabilityScores[j].Dimension = strings.TrimSpace(req.Evaluations[i].CapabilityScores[j].Dimension)
			req.Evaluations[i].CapabilityScores[j].Comment = strings.TrimSpace(req.Evaluations[i].CapabilityScores[j].Comment)
		}
		sort.Slice(req.Evaluations[i].CapabilityScores, func(a, b int) bool {
			return req.Evaluations[i].CapabilityScores[a].Dimension < req.Evaluations[i].CapabilityScores[b].Dimension
		})
	}

	sort.Slice(req.Evaluations, func(i, j int) bool {
		left := req.Evaluations[i]
		right := req.Evaluations[j]
		if compareRounds(left.Round, right.Round) != 0 {
			return compareRounds(left.Round, right.Round) < 0
		}
		if left.InterviewID != right.InterviewID {
			return left.InterviewID < right.InterviewID
		}
		if left.InterviewerID != right.InterviewerID {
			return left.InterviewerID < right.InterviewerID
		}
		if !left.SubmittedAt.Equal(right.SubmittedAt) {
			return left.SubmittedAt.Before(right.SubmittedAt)
		}
		return left.Conclusion < right.Conclusion
	})

	return req
}

func validateGenerateRequest(req GenerateRequest) ValidationErrors {
	var errs ValidationErrors
	if req.Candidate.ID == "" {
		errs = append(errs, ValidationError{Field: "candidate.id", Message: "is required"})
	}
	if req.Candidate.FullName == "" {
		errs = append(errs, ValidationError{Field: "candidate.full_name", Message: "is required"})
	}
	if len(req.Evaluations) == 0 {
		errs = append(errs, ValidationError{Field: "evaluations", Message: "must contain at least one latest evaluation"})
	}
	for i, item := range req.Evaluations {
		prefix := fmt.Sprintf("evaluations[%d]", i)
		if item.InterviewID == "" {
			errs = append(errs, ValidationError{Field: prefix + ".interview_id", Message: "is required"})
		}
		if item.Round == "" {
			errs = append(errs, ValidationError{Field: prefix + ".round", Message: "is required"})
		}
		if item.InterviewerID == "" {
			errs = append(errs, ValidationError{Field: prefix + ".interviewer_id", Message: "is required"})
		}
		if item.AverageScore < 1 || item.AverageScore > 5 {
			errs = append(errs, ValidationError{Field: prefix + ".average_score", Message: "must be between 1 and 5"})
		}
		if len(item.CapabilityScores) == 0 {
			errs = append(errs, ValidationError{Field: prefix + ".capability_scores", Message: "must contain at least one score"})
		}
		for j, score := range item.CapabilityScores {
			scorePrefix := fmt.Sprintf("%s.capability_scores[%d]", prefix, j)
			if score.Dimension == "" {
				errs = append(errs, ValidationError{Field: scorePrefix + ".dimension", Message: "is required"})
			}
			if score.Score < 1 || score.Score > 5 {
				errs = append(errs, ValidationError{Field: scorePrefix + ".score", Message: "must be between 1 and 5"})
			}
		}
	}
	return errs
}

func deterministicReportID(req GenerateRequest) string {
	data, _ := json.Marshal(req)
	hash := sha256.Sum256(data)
	return "rpt_" + hex.EncodeToString(hash[:8])
}

func buildFinalComment(parts []string) string {
	if len(parts) == 0 {
		return "No interviewer comments were provided."
	}
	return strings.Join(parts, " | ")
}

func recommendHire(avg float64) string {
	switch {
	case avg >= 4.2:
		return "strong_hire"
	case avg >= 3.5:
		return "hire"
	case avg >= 2.8:
		return "hold"
	default:
		return "no_hire"
	}
}

func compareRounds(left, right string) int {
	leftNum := parseRoundNumber(left)
	rightNum := parseRoundNumber(right)
	switch {
	case leftNum > 0 && rightNum > 0 && leftNum != rightNum:
		if leftNum < rightNum {
			return -1
		}
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func parseRoundNumber(raw string) int {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, "round-")
	n, _ := strconv.Atoi(value)
	return n
}

func buildMarkdown(report StructuredInterviewReport) string {
	var b strings.Builder
	b.WriteString("# Structured Interview Report\n\n")
	b.WriteString(fmt.Sprintf("- Report ID: `%s`\n", report.ReportID))
	b.WriteString(fmt.Sprintf("- Candidate: %s (%s)\n", report.Candidate.FullName, report.Candidate.ID))
	if report.Candidate.CurrentTitle != "" {
		b.WriteString(fmt.Sprintf("- Current Title: %s\n", report.Candidate.CurrentTitle))
	}
	if report.Candidate.CurrentCompany != "" {
		b.WriteString(fmt.Sprintf("- Current Company: %s\n", report.Candidate.CurrentCompany))
	}
	b.WriteString(fmt.Sprintf("- Generated At: %s\n", report.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Average Score: %.2f\n", report.AverageScore))
	b.WriteString(fmt.Sprintf("- Hiring Recommendation: **%s**\n\n", report.HiringRecommendation))

	b.WriteString("## Score Details\n\n")
	b.WriteString("| Round | Interview | Interviewer | Avg Score | Conclusion | Dimension | Score | Comment |\n")
	b.WriteString("|---|---|---|---:|---|---|---:|---|\n")
	for _, item := range report.Scores {
		for _, dim := range item.CapabilityScores {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %.2f | %s | %s | %d | %s |\n",
				item.Round,
				item.InterviewID,
				item.InterviewerID,
				item.AverageScore,
				item.Conclusion,
				dim.Dimension,
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

func escapePipe(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func round(value float64, places int) float64 {
	if places <= 0 {
		return float64(int(value + 0.5))
	}
	f := 1.0
	for i := 0; i < places; i++ {
		f *= 10
	}
	return float64(int(value*f+0.5)) / f
}

func cloneReport(in StructuredInterviewReport) StructuredInterviewReport {
	out := in
	out.Scores = make([]EvaluationScore, 0, len(in.Scores))
	for _, item := range in.Scores {
		cloned := item
		cloned.CapabilityScores = append([]CapabilityScore(nil), item.CapabilityScores...)
		out.Scores = append(out.Scores, cloned)
	}
	if in.ConclusionBreakdown != nil {
		out.ConclusionBreakdown = make(map[string]int, len(in.ConclusionBreakdown))
		for key, value := range in.ConclusionBreakdown {
			out.ConclusionBreakdown[key] = value
		}
	}
	return out
}
