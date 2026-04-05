package report

import "time"

type CandidateSnapshot struct {
	ID             string `json:"id"`
	FullName       string `json:"full_name"`
	Email          string `json:"email,omitempty"`
	Phone          string `json:"phone,omitempty"`
	CurrentTitle   string `json:"current_title,omitempty"`
	CurrentCompany string `json:"current_company,omitempty"`
	StatusLayer    string `json:"status_layer,omitempty"`
}

type CapabilityScore struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"`
	Comment   string `json:"comment,omitempty"`
}

type EvaluationSnapshot struct {
	InterviewID      string            `json:"interview_id"`
	Round            string            `json:"round"`
	InterviewerID    string            `json:"interviewer_id"`
	AverageScore     float64           `json:"average_score"`
	CapabilityScores []CapabilityScore `json:"capability_scores"`
	OverallComment   string            `json:"overall_comment"`
	Conclusion       string            `json:"conclusion"`
	SubmittedAt      time.Time         `json:"submitted_at"`
}

type GenerateRequest struct {
	Candidate   CandidateSnapshot    `json:"candidate"`
	Evaluations []EvaluationSnapshot `json:"evaluations"`
	GeneratedBy string               `json:"generated_by,omitempty"`
}

type EvaluationScore struct {
	InterviewID      string            `json:"interview_id"`
	Round            string            `json:"round"`
	InterviewerID    string            `json:"interviewer_id"`
	AverageScore     float64           `json:"average_score"`
	CapabilityScores []CapabilityScore `json:"capability_scores"`
	OverallComment   string            `json:"overall_comment"`
	Conclusion       string            `json:"conclusion"`
	SubmittedAt      time.Time         `json:"submitted_at"`
}

type DimensionScorePoint struct {
	Round         string  `json:"round"`
	InterviewID   string  `json:"interview_id"`
	InterviewerID string  `json:"interviewer_id"`
	Score         float64 `json:"score"`
}

type DimensionComparison struct {
	Dimension    string                `json:"dimension"`
	AverageScore float64               `json:"average_score"`
	HighestScore float64               `json:"highest_score"`
	LowestScore  float64               `json:"lowest_score"`
	Scores       []DimensionScorePoint `json:"scores"`
}

type RadarDimension struct {
	Dimension string  `json:"dimension"`
	MaxScore  float64 `json:"max_score"`
}

type RadarDimensionValue struct {
	Dimension string  `json:"dimension"`
	Score     float64 `json:"score"`
}

type RadarSeries struct {
	Round         string                `json:"round"`
	InterviewID   string                `json:"interview_id"`
	InterviewerID string                `json:"interviewer_id"`
	Values        []RadarDimensionValue `json:"values"`
}

type RadarChartData struct {
	Dimensions []RadarDimension `json:"dimensions"`
	Series     []RadarSeries    `json:"series"`
}

type StructuredInterviewReport struct {
	ReportID              string                `json:"report_id"`
	Candidate             CandidateSnapshot     `json:"candidate"`
	Scores                []EvaluationScore     `json:"scores"`
	DimensionComparisons  []DimensionComparison `json:"dimension_comparisons"`
	RadarChart            RadarChartData        `json:"radar_chart"`
	FinalComment          string                `json:"final_comment"`
	HiringRecommendation  string                `json:"hiring_recommendation"`
	AverageScore          float64               `json:"average_score"`
	ConclusionBreakdown   map[string]int        `json:"conclusion_breakdown"`
	SourceEvaluationCount int                   `json:"source_evaluation_count"`
	GeneratedBy           string                `json:"generated_by,omitempty"`
	GeneratedAt           time.Time             `json:"generated_at"`
}

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	if len(v) == 0 {
		return "validation failed"
	}
	out := "validation failed: "
	for i, item := range v {
		if i > 0 {
			out += "; "
		}
		out += item.Field + " " + item.Message
	}
	return out
}
