package analytics

type StageMetric struct {
	Stage          string  `json:"stage"`
	CandidateCount int     `json:"candidate_count"`
	ConversionRate float64 `json:"conversion_rate"`
}

type FunnelMetrics struct {
	TotalCandidates int           `json:"total_candidates"`
	Stages          []StageMetric `json:"stages"`
}

type InterviewerWorkload struct {
	InterviewerID            string  `json:"interviewer_id"`
	FeedbackCount            int     `json:"feedback_count"`
	AvgFeedbackDurationHours float64 `json:"avg_feedback_duration_hours"`
}

type EfficiencyMetrics struct {
	TotalFeedbackCount       int                   `json:"total_feedback_count"`
	AvgFeedbackDurationHours float64               `json:"avg_feedback_duration_hours"`
	InterviewerWorkload      []InterviewerWorkload `json:"interviewer_workload"`
}

type MetricDefinition struct {
	MetricID    string `json:"metric_id"`
	Name        string `json:"name"`
	Definition  string `json:"definition"`
	Formula     string `json:"formula"`
	Unit        string `json:"unit"`
	Aggregation string `json:"aggregation"`
}

type Dashboard struct {
	Funnel            FunnelMetrics      `json:"funnel"`
	Efficiency        EfficiencyMetrics  `json:"efficiency"`
	MetricDefinitions []MetricDefinition `json:"metric_definitions"`
}
