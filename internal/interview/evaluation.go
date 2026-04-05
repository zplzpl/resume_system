package interview

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrEvaluationInterviewerNotAssigned = errors.New("interviewer is not assigned to this interview")
)

type EvaluationConclusion string

const (
	EvaluationConclusionStrongHire EvaluationConclusion = "strong_hire"
	EvaluationConclusionHire       EvaluationConclusion = "hire"
	EvaluationConclusionHold       EvaluationConclusion = "hold"
	EvaluationConclusionNoHire     EvaluationConclusion = "no_hire"
)

var evaluationTemplate = []string{
	"technical_depth",
	"problem_solving",
	"communication",
	"collaboration",
}

type CapabilityScore struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"`
	Comment   string `json:"comment,omitempty"`
}

type Evaluation struct {
	ID               string               `json:"id"`
	InterviewID      string               `json:"interview_id"`
	CandidateID      string               `json:"candidate_id"`
	Round            string               `json:"round"`
	InterviewerID    string               `json:"interviewer_id"`
	Version          int                  `json:"version"`
	CapabilityScores []CapabilityScore    `json:"capability_scores"`
	OverallComment   string               `json:"overall_comment"`
	Conclusion       EvaluationConclusion `json:"conclusion"`
	AverageScore     float64              `json:"average_score"`
	SubmittedAt      time.Time            `json:"submitted_at"`
	ArchivedAt       *time.Time           `json:"archived_at,omitempty"`
	IsLatest         bool                 `json:"is_latest"`
}

type SubmitEvaluationRequest struct {
	CapabilityScores []CapabilityScore
	OverallComment   string
	Conclusion       string
}

type CandidateLatestEvaluationsView struct {
	CandidateID         string                       `json:"candidate_id"`
	Rounds              []RoundEvaluationView        `json:"rounds"`
	LatestEvaluations   []Evaluation                 `json:"latest_evaluations"`
	TotalLatestCount    int                          `json:"total_latest_count"`
	OverallAverageScore float64                      `json:"overall_average_score"`
	ConclusionBreakdown map[EvaluationConclusion]int `json:"conclusion_breakdown"`
	GeneratedAt         time.Time                    `json:"generated_at"`
}

type RoundEvaluationView struct {
	Round               string                       `json:"round"`
	Evaluations         []Evaluation                 `json:"evaluations"`
	AverageScore        float64                      `json:"average_score"`
	ConclusionBreakdown map[EvaluationConclusion]int `json:"conclusion_breakdown"`
}

func ParseEvaluationConclusion(raw string) (EvaluationConclusion, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch EvaluationConclusion(value) {
	case EvaluationConclusionStrongHire:
		return EvaluationConclusionStrongHire, nil
	case EvaluationConclusionHire:
		return EvaluationConclusionHire, nil
	case EvaluationConclusionHold:
		return EvaluationConclusionHold, nil
	case EvaluationConclusionNoHire:
		return EvaluationConclusionNoHire, nil
	default:
		return "", fmt.Errorf("invalid conclusion: %q", raw)
	}
}

func (s *Service) SubmitEvaluation(interviewID, interviewerID string, req SubmitEvaluationRequest) (Evaluation, error) {
	interviewID = strings.TrimSpace(interviewID)
	item, ok := s.repo.GetInterview(interviewID)
	if !ok {
		return Evaluation{}, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}

	interviewerID = strings.TrimSpace(interviewerID)
	if interviewerID == "" {
		return Evaluation{}, fmt.Errorf("interviewer_id is required")
	}
	if !containsID(item.InterviewerIDs, interviewerID) {
		return Evaluation{}, ErrEvaluationInterviewerNotAssigned
	}

	scores, averageScore, err := normalizeCapabilityScores(req.CapabilityScores)
	if err != nil {
		return Evaluation{}, err
	}

	overallComment := strings.TrimSpace(req.OverallComment)
	if overallComment == "" {
		return Evaluation{}, fmt.Errorf("overall_comment is required")
	}

	conclusion, err := ParseEvaluationConclusion(req.Conclusion)
	if err != nil {
		return Evaluation{}, err
	}

	return s.repo.AddEvaluation(Evaluation{
		InterviewID:      item.ID,
		CandidateID:      item.CandidateID,
		Round:            item.Round,
		InterviewerID:    interviewerID,
		CapabilityScores: scores,
		OverallComment:   overallComment,
		Conclusion:       conclusion,
		AverageScore:     averageScore,
	}), nil
}

func (s *Service) ListEvaluations(interviewID string) ([]Evaluation, error) {
	interviewID = strings.TrimSpace(interviewID)
	if _, ok := s.repo.GetInterview(interviewID); !ok {
		return nil, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}
	return s.repo.ListEvaluationsByInterview(interviewID), nil
}

func (s *Service) BuildCandidateLatestEvaluationsView(candidateID string) CandidateLatestEvaluationsView {
	latest := s.repo.ListLatestEvaluationsByCandidate(candidateID)
	roundBuckets := make(map[string][]Evaluation)
	conclusionBreakdown := make(map[EvaluationConclusion]int)
	totalScore := 0.0

	for _, item := range latest {
		roundBuckets[item.Round] = append(roundBuckets[item.Round], item)
		conclusionBreakdown[item.Conclusion]++
		totalScore += item.AverageScore
	}

	rounds := make([]string, 0, len(roundBuckets))
	for round := range roundBuckets {
		rounds = append(rounds, round)
	}
	sort.Slice(rounds, func(i, j int) bool {
		return compareRounds(rounds[i], rounds[j]) < 0
	})

	roundViews := make([]RoundEvaluationView, 0, len(rounds))
	for _, round := range rounds {
		items := roundBuckets[round]
		sort.Slice(items, func(i, j int) bool {
			if items[i].SubmittedAt.Equal(items[j].SubmittedAt) {
				return items[i].InterviewerID < items[j].InterviewerID
			}
			return items[i].SubmittedAt.After(items[j].SubmittedAt)
		})

		roundConclusion := make(map[EvaluationConclusion]int)
		roundScore := 0.0
		for _, item := range items {
			roundConclusion[item.Conclusion]++
			roundScore += item.AverageScore
		}

		roundViews = append(roundViews, RoundEvaluationView{
			Round:               round,
			Evaluations:         items,
			AverageScore:        roundScore / float64(len(items)),
			ConclusionBreakdown: roundConclusion,
		})
	}

	overallAverage := 0.0
	if len(latest) > 0 {
		overallAverage = totalScore / float64(len(latest))
	}

	return CandidateLatestEvaluationsView{
		CandidateID:         candidateID,
		Rounds:              roundViews,
		LatestEvaluations:   latest,
		TotalLatestCount:    len(latest),
		OverallAverageScore: overallAverage,
		ConclusionBreakdown: conclusionBreakdown,
		GeneratedAt:         time.Now().UTC(),
	}
}

func normalizeCapabilityScores(raw []CapabilityScore) ([]CapabilityScore, float64, error) {
	if len(raw) != len(evaluationTemplate) {
		return nil, 0, fmt.Errorf("capability_scores must include %d template dimensions", len(evaluationTemplate))
	}

	templateOrder := make(map[string]int, len(evaluationTemplate))
	templateSet := make(map[string]struct{}, len(evaluationTemplate))
	for index, dimension := range evaluationTemplate {
		templateOrder[dimension] = index
		templateSet[dimension] = struct{}{}
	}

	scores := make([]CapabilityScore, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	total := 0
	for _, item := range raw {
		dimension := strings.ToLower(strings.TrimSpace(item.Dimension))
		if _, ok := templateSet[dimension]; !ok {
			return nil, 0, fmt.Errorf("unsupported capability dimension: %q", item.Dimension)
		}
		if _, ok := seen[dimension]; ok {
			return nil, 0, fmt.Errorf("duplicate capability dimension: %q", item.Dimension)
		}
		if item.Score < 1 || item.Score > 5 {
			return nil, 0, fmt.Errorf("capability score must be between 1 and 5")
		}
		seen[dimension] = struct{}{}
		total += item.Score
		scores = append(scores, CapabilityScore{
			Dimension: dimension,
			Score:     item.Score,
			Comment:   strings.TrimSpace(item.Comment),
		})
	}

	if len(seen) != len(templateSet) {
		missing := make([]string, 0)
		for _, dimension := range evaluationTemplate {
			if _, ok := seen[dimension]; !ok {
				missing = append(missing, dimension)
			}
		}
		return nil, 0, fmt.Errorf("missing capability dimensions: %s", strings.Join(missing, ","))
	}

	sort.Slice(scores, func(i, j int) bool {
		return templateOrder[scores[i].Dimension] < templateOrder[scores[j].Dimension]
	})

	average := float64(total) / float64(len(scores))
	average = math.Round(average*100) / 100
	return scores, average, nil
}

func containsID(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func compareRounds(left, right string) int {
	lNum := parseRoundNumber(left)
	rNum := parseRoundNumber(right)
	switch {
	case lNum > 0 && rNum > 0 && lNum != rNum:
		if lNum < rNum {
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

func parseRoundNumber(round string) int {
	value := strings.TrimSpace(strings.ToLower(round))
	if value == "" {
		return 0
	}
	value = strings.TrimPrefix(value, "round-")
	num, _ := strconv.Atoi(value)
	return num
}
