package report

import "time"

type CandidateInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Phone    string `json:"phone,omitempty"`
	Position string `json:"position"`
}

type ScoreDimension struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	Comment string  `json:"comment,omitempty"`
}

type InterviewEvaluation struct {
	InterviewID     string           `json:"interview_id"`
	InterviewerID   string           `json:"interviewer_id"`
	InterviewerName string           `json:"interviewer_name"`
	OverallScore    float64          `json:"overall_score"`
	Summary         string           `json:"summary"`
	Dimensions      []ScoreDimension `json:"dimensions"`
}

type GenerateRequest struct {
	Candidate   CandidateInfo         `json:"candidate"`
	Evaluations []InterviewEvaluation `json:"evaluations"`
	GeneratedBy string                `json:"generated_by,omitempty"`
}

type InterviewerScore struct {
	InterviewID     string           `json:"interview_id"`
	InterviewerID   string           `json:"interviewer_id"`
	InterviewerName string           `json:"interviewer_name"`
	OverallScore    float64          `json:"overall_score"`
	Dimensions      []ScoreDimension `json:"dimensions"`
	Summary         string           `json:"summary"`
}

type StructuredInterviewReport struct {
	ReportID             string             `json:"report_id"`
	Candidate            CandidateInfo      `json:"candidate"`
	Scores               []InterviewerScore `json:"scores"`
	FinalComment         string             `json:"final_comment"`
	HiringRecommendation string             `json:"hiring_recommendation"`
	AverageScore         float64            `json:"average_score"`
	GeneratedBy          string             `json:"generated_by,omitempty"`
	GeneratedAt          time.Time          `json:"generated_at"`
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
