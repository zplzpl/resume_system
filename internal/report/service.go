package report

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
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

	averageScore := total / float64(len(req.Evaluations))
	recommendation := recommendHire(averageScore)
	reportID := deterministicReportID(req)
	report := StructuredInterviewReport{
		ReportID:             reportID,
		Candidate:            req.Candidate,
		Scores:               scores,
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
	case "xlsx":
		xlsxData, xlsxErr := buildXLSX(report)
		if xlsxErr != nil {
			return "", "", nil, xlsxErr
		}
		return fmt.Sprintf("interview-report-%s.xlsx", reportID), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", xlsxData, nil
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

func buildXLSX(report StructuredInterviewReport) ([]byte, error) {
	rows := [][]string{
		{
			"report_id",
			"generated_at",
			"generated_by",
			"candidate_id",
			"candidate_name",
			"candidate_email",
			"candidate_phone",
			"candidate_position",
			"hiring_recommendation",
			"average_score",
			"interview_id",
			"interviewer_id",
			"interviewer_name",
			"overall_score",
			"dimension_name",
			"dimension_score",
			"dimension_comment",
			"summary",
			"final_comment",
		},
	}

	for _, score := range report.Scores {
		for _, dim := range score.Dimensions {
			rows = append(rows, []string{
				report.ReportID,
				report.GeneratedAt.Format(time.RFC3339),
				report.GeneratedBy,
				report.Candidate.ID,
				report.Candidate.Name,
				report.Candidate.Email,
				report.Candidate.Phone,
				report.Candidate.Position,
				report.HiringRecommendation,
				fmt.Sprintf("%.2f", report.AverageScore),
				score.InterviewID,
				score.InterviewerID,
				score.InterviewerName,
				fmt.Sprintf("%.2f", score.OverallScore),
				dim.Name,
				fmt.Sprintf("%.2f", dim.Score),
				dim.Comment,
				score.Summary,
				report.FinalComment,
			})
		}
	}

	var workbook bytes.Buffer
	archive := zip.NewWriter(&workbook)
	writeEntry := func(name, content string) error {
		f, err := archive.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write([]byte(content))
		return err
	}

	if err := writeEntry("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`); err != nil {
		return nil, err
	}
	if err := writeEntry("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`); err != nil {
		return nil, err
	}
	if err := writeEntry("xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"
 xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Report" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`); err != nil {
		return nil, err
	}
	if err := writeEntry("xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`); err != nil {
		return nil, err
	}
	if err := writeEntry("xl/worksheets/sheet1.xml", buildSheetXML(rows)); err != nil {
		return nil, err
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return workbook.Bytes(), nil
}

func buildSheetXML(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString("\n")
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	b.WriteString("\n  <sheetData>\n")
	for rowIndex, row := range rows {
		b.WriteString(fmt.Sprintf("    <row r=\"%d\">", rowIndex+1))
		for _, cell := range row {
			b.WriteString(`<c t="inlineStr"><is><t xml:space="preserve">`)
			b.WriteString(escapeXML(cell))
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString("</row>\n")
	}
	b.WriteString("  </sheetData>\n")
	b.WriteString("</worksheet>")
	return b.String()
}

func escapeXML(in string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(in))
	return b.String()
}

func escapePipe(in string) string {
	return strings.ReplaceAll(in, "|", "\\|")
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
