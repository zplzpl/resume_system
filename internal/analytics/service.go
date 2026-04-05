package analytics

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/zplzpl/resume_system/internal/interview"
	"github.com/zplzpl/resume_system/internal/resume"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) BuildDashboard(candidates []resume.CandidateProfile, interviews []interview.Interview, evaluations []interview.Evaluation) Dashboard {
	stageOrder := []resume.CandidateStatusLayer{
		resume.CandidateStatusNew,
		resume.CandidateStatusScreening,
		resume.CandidateStatusInterview,
		resume.CandidateStatusOffer,
		resume.CandidateStatusHired,
	}

	stageCount := make(map[resume.CandidateStatusLayer]int, len(stageOrder))
	for _, stage := range stageOrder {
		stageCount[stage] = 0
	}
	for _, candidate := range candidates {
		if _, ok := stageCount[candidate.StatusLayer]; !ok {
			continue
		}
		stageCount[candidate.StatusLayer]++
	}

	totalCandidates := 0
	stages := make([]StageMetric, 0, len(stageOrder))
	prevStageCount := 0
	for idx, stage := range stageOrder {
		count := stageCount[stage]
		totalCandidates += count

		conversion := 0.0
		if idx == 0 {
			if count > 0 {
				conversion = 1
			}
		} else if prevStageCount > 0 {
			conversion = float64(count) / float64(prevStageCount)
		}

		stages = append(stages, StageMetric{
			Stage:          string(stage),
			CandidateCount: count,
			ConversionRate: round2(conversion),
		})
		prevStageCount = count
	}

	interviewByID := make(map[string]interview.Interview, len(interviews))
	for _, item := range interviews {
		interviewByID[item.ID] = item
	}

	type workloadAccumulator struct {
		interviewerID string
		count         int
		totalHours    float64
	}

	workloadByInterviewer := make(map[string]*workloadAccumulator)
	totalFeedbackCount := 0
	totalFeedbackHours := 0.0

	for _, evaluation := range evaluations {
		item, ok := interviewByID[evaluation.InterviewID]
		if !ok {
			continue
		}

		durationHours := evaluation.SubmittedAt.Sub(item.EndsAt).Hours()
		if durationHours < 0 {
			durationHours = 0
		}

		interviewerID := strings.TrimSpace(evaluation.InterviewerID)
		if interviewerID == "" {
			interviewerID = "unknown"
		}

		acc, ok := workloadByInterviewer[interviewerID]
		if !ok {
			acc = &workloadAccumulator{interviewerID: interviewerID}
			workloadByInterviewer[interviewerID] = acc
		}
		acc.count++
		acc.totalHours += durationHours

		totalFeedbackCount++
		totalFeedbackHours += durationHours
	}

	workloads := make([]InterviewerWorkload, 0, len(workloadByInterviewer))
	for _, acc := range workloadByInterviewer {
		avgHours := 0.0
		if acc.count > 0 {
			avgHours = acc.totalHours / float64(acc.count)
		}
		workloads = append(workloads, InterviewerWorkload{
			InterviewerID:            acc.interviewerID,
			FeedbackCount:            acc.count,
			AvgFeedbackDurationHours: round2(avgHours),
		})
	}
	sort.Slice(workloads, func(i, j int) bool {
		if workloads[i].FeedbackCount == workloads[j].FeedbackCount {
			return workloads[i].InterviewerID < workloads[j].InterviewerID
		}
		return workloads[i].FeedbackCount > workloads[j].FeedbackCount
	})

	avgFeedbackHours := 0.0
	if totalFeedbackCount > 0 {
		avgFeedbackHours = totalFeedbackHours / float64(totalFeedbackCount)
	}

	return Dashboard{
		Funnel: FunnelMetrics{
			TotalCandidates: totalCandidates,
			Stages:          stages,
		},
		Efficiency: EfficiencyMetrics{
			TotalFeedbackCount:       totalFeedbackCount,
			AvgFeedbackDurationHours: round2(avgFeedbackHours),
			InterviewerWorkload:      workloads,
		},
		MetricDefinitions: []MetricDefinition{
			{
				MetricID:    "stage_candidate_count",
				Name:        "阶段人数",
				Definition:  "每个招聘阶段的当前候选人数",
				Formula:     "count(candidates where status_layer = stage)",
				Unit:        "人",
				Aggregation: "按阶段",
			},
			{
				MetricID:    "stage_conversion_rate",
				Name:        "阶段转化率",
				Definition:  "当前阶段人数相对于上一阶段人数的比率",
				Formula:     "stage_count(current_stage) / stage_count(previous_stage)",
				Unit:        "比例",
				Aggregation: "按阶段",
			},
			{
				MetricID:    "interviewer_workload",
				Name:        "面试官工作量",
				Definition:  "统计周期内每位面试官提交的反馈数量",
				Formula:     "count(evaluations where interviewer_id = x)",
				Unit:        "条反馈",
				Aggregation: "按面试官",
			},
			{
				MetricID:    "feedback_duration_hours",
				Name:        "反馈时长",
				Definition:  "从面试结束到反馈提交的耗时",
				Formula:     "evaluation.submitted_at - interview.ends_at",
				Unit:        "小时",
				Aggregation: "平均值",
			},
		},
	}
}

func (s *Service) BuildDashboardCSV(dashboard Dashboard) ([]byte, error) {
	buf := &bytes.Buffer{}
	writer := csv.NewWriter(buf)

	if err := writer.Write([]string{"section", "metric", "value", "note"}); err != nil {
		return nil, fmt.Errorf("write csv header: %w", err)
	}

	if err := writer.Write([]string{"funnel", "total_candidates", fmt.Sprintf("%d", dashboard.Funnel.TotalCandidates), "漏斗总人数（不含 archived）"}); err != nil {
		return nil, fmt.Errorf("write funnel total: %w", err)
	}
	for _, stage := range dashboard.Funnel.Stages {
		countMetric := fmt.Sprintf("%s_candidate_count", stage.Stage)
		if err := writer.Write([]string{"funnel", countMetric, fmt.Sprintf("%d", stage.CandidateCount), "阶段人数"}); err != nil {
			return nil, fmt.Errorf("write stage candidate count: %w", err)
		}
		conversionMetric := fmt.Sprintf("%s_conversion_rate", stage.Stage)
		if err := writer.Write([]string{"funnel", conversionMetric, fmt.Sprintf("%.2f", stage.ConversionRate), "相对上一阶段"}); err != nil {
			return nil, fmt.Errorf("write stage conversion rate: %w", err)
		}
	}

	if err := writer.Write([]string{"efficiency", "total_feedback_count", fmt.Sprintf("%d", dashboard.Efficiency.TotalFeedbackCount), "反馈总数"}); err != nil {
		return nil, fmt.Errorf("write efficiency total feedback: %w", err)
	}
	if err := writer.Write([]string{"efficiency", "avg_feedback_duration_hours", fmt.Sprintf("%.2f", dashboard.Efficiency.AvgFeedbackDurationHours), "平均反馈时长"}); err != nil {
		return nil, fmt.Errorf("write efficiency average feedback duration: %w", err)
	}
	for _, workload := range dashboard.Efficiency.InterviewerWorkload {
		countMetric := fmt.Sprintf("interviewer_%s_feedback_count", workload.InterviewerID)
		if err := writer.Write([]string{"efficiency", countMetric, fmt.Sprintf("%d", workload.FeedbackCount), "面试官工作量"}); err != nil {
			return nil, fmt.Errorf("write interviewer workload count: %w", err)
		}
		avgMetric := fmt.Sprintf("interviewer_%s_avg_feedback_duration_hours", workload.InterviewerID)
		if err := writer.Write([]string{"efficiency", avgMetric, fmt.Sprintf("%.2f", workload.AvgFeedbackDurationHours), "面试官平均反馈时长"}); err != nil {
			return nil, fmt.Errorf("write interviewer workload average: %w", err)
		}
	}

	for _, definition := range dashboard.MetricDefinitions {
		note := fmt.Sprintf("%s | %s | %s", definition.Unit, definition.Aggregation, definition.Formula)
		if err := writer.Write([]string{"definition", definition.MetricID, definition.Definition, note}); err != nil {
			return nil, fmt.Errorf("write metric definition: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush csv writer: %w", err)
	}

	return buf.Bytes(), nil
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
