package interview

import (
	"fmt"
	"strings"
	"time"
)

type QuestionGenerator interface {
	Generate(candidate CandidateSnapshot, item Interview, req GenerateQuestionRecommendationRequest) ([]RecommendedQuestion, error)
}

type DefaultQuestionGenerator struct{}

func NewDefaultQuestionGenerator() QuestionGenerator {
	return &DefaultQuestionGenerator{}
}

func (g *DefaultQuestionGenerator) Generate(candidate CandidateSnapshot, item Interview, req GenerateQuestionRecommendationRequest) ([]RecommendedQuestion, error) {
	jobDescription := strings.TrimSpace(req.JobDescription)
	if jobDescription == "" {
		return nil, fmt.Errorf("job_description is required for AI generation")
	}

	jobTitle := strings.TrimSpace(req.JobTitle)
	if jobTitle == "" {
		jobTitle = "the target role"
	}

	lowerJD := strings.ToLower(jobDescription)
	questions := make([]RecommendedQuestion, 0, 6)

	experienceAnchor := candidate.CurrentCompany
	if experienceAnchor == "" {
		experienceAnchor = "your recent experience"
	}
	roleAnchor := candidate.CurrentTitle
	if roleAnchor == "" {
		roleAnchor = "previous roles"
	}

	questions = append(questions, RecommendedQuestion{
		Category:  QuestionCategoryExperienceFollowUp,
		Question:  fmt.Sprintf("In %s, what was the most complex problem you solved and how did you drive it to delivery?", experienceAnchor),
		Focus:     "experience-depth",
		Reference: "resume:current_company",
	})
	questions = append(questions, RecommendedQuestion{
		Category:  QuestionCategoryExperienceFollowUp,
		Question:  fmt.Sprintf("When working as %s, which tradeoff decision had the highest impact on business outcome?", roleAnchor),
		Focus:     "decision-making",
		Reference: "resume:current_title",
	})
	if candidate.TotalExperienceMonths > 0 {
		questions = append(questions, RecommendedQuestion{
			Category:  QuestionCategoryExperienceFollowUp,
			Question:  fmt.Sprintf("Across your %s of experience, how has your approach to risk management evolved?", formatExperience(candidate.TotalExperienceMonths)),
			Focus:     "growth-trajectory",
			Reference: "resume:total_experience_months",
		})
	}

	matchedSkills := matchSkills(candidate.Skills, lowerJD)
	if len(matchedSkills) > 0 {
		questions = append(questions, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  fmt.Sprintf("This role emphasizes %s. Describe one production scenario where you applied it under pressure.", strings.Join(matchedSkills, ", ")),
			Focus:     "job-skill-fit",
			Reference: "jd+resume:skills",
		})
	}

	if strings.Contains(lowerJD, "architecture") || strings.Contains(lowerJD, "system design") {
		questions = append(questions, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  fmt.Sprintf("For the %s position, walk through a scalable architecture you would design and the key tradeoffs.", jobTitle),
			Focus:     "architecture",
			Reference: "jd:architecture",
		})
	}
	if strings.Contains(lowerJD, "collaboration") || strings.Contains(lowerJD, "cross-functional") || strings.Contains(lowerJD, "stakeholder") {
		questions = append(questions, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  "Tell me about a cross-functional disagreement you handled and what changed after your intervention.",
			Focus:     "collaboration",
			Reference: "jd:collaboration",
		})
	}
	if strings.Contains(lowerJD, "performance") || strings.Contains(lowerJD, "reliability") || strings.Contains(lowerJD, "quality") {
		questions = append(questions, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  "Describe a reliability or performance incident you diagnosed; how did you verify root cause and prevent recurrence?",
			Focus:     "quality-reliability",
			Reference: "jd:quality",
		})
	}

	questions = ensureQuestionCoverage(questions, candidate, jobTitle)
	return questions, nil
}

func (s *Service) GenerateQuestionRecommendation(interviewID string, candidate CandidateSnapshot, req GenerateQuestionRecommendationRequest) (QuestionRecommendation, error) {
	interviewID = strings.TrimSpace(interviewID)
	item, ok := s.repo.GetInterview(interviewID)
	if !ok {
		return QuestionRecommendation{}, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}

	candidate.ID = strings.TrimSpace(candidate.ID)
	if candidate.ID == "" {
		candidate.ID = item.CandidateID
	}
	if candidate.ID != item.CandidateID {
		return QuestionRecommendation{}, fmt.Errorf("candidate mismatch for interview: interview=%s, candidate=%s", item.CandidateID, candidate.ID)
	}
	candidate.FullName = strings.TrimSpace(candidate.FullName)
	candidate.CurrentCompany = strings.TrimSpace(candidate.CurrentCompany)
	candidate.CurrentTitle = strings.TrimSpace(candidate.CurrentTitle)
	candidate.HighestEducation = strings.TrimSpace(candidate.HighestEducation)
	candidate.Skills = dedupNonEmpty(candidate.Skills)

	req.JobTitle = strings.TrimSpace(req.JobTitle)
	req.JobDescription = strings.TrimSpace(req.JobDescription)

	questions, generationErr := s.questionGen.Generate(candidate, item, req)

	recommendation := QuestionRecommendation{
		InterviewID:    item.ID,
		CandidateID:    item.CandidateID,
		Round:          item.Round,
		JobTitle:       req.JobTitle,
		JobDescription: req.JobDescription,
		GeneratedAt:    time.Now().UTC(),
	}

	if generationErr != nil {
		recommendation.Questions = buildFallbackQuestions(candidate, req.JobTitle)
		recommendation.FallbackUsed = true
		recommendation.FallbackReason = generationErr.Error()
		recommendation.GeneratedSource = "template_fallback"
	} else {
		recommendation.Questions = questions
		recommendation.GeneratedSource = "ai_synthesizer"
	}

	recommendation.Questions = trimQuestions(recommendation.Questions, 8)
	if len(recommendation.Questions) == 0 {
		return QuestionRecommendation{}, fmt.Errorf("failed to build recommendation questions")
	}

	s.repo.SaveQuestionRecommendation(recommendation)
	return recommendation, nil
}

func (s *Service) GetQuestionRecommendation(interviewID string) (QuestionRecommendation, error) {
	interviewID = strings.TrimSpace(interviewID)
	if _, ok := s.repo.GetInterview(interviewID); !ok {
		return QuestionRecommendation{}, fmt.Errorf("%w: %s", ErrInterviewNotFound, interviewID)
	}

	item, ok := s.repo.GetQuestionRecommendation(interviewID)
	if !ok {
		return QuestionRecommendation{}, fmt.Errorf("%w: %s", ErrQuestionRecommendationNotFound, interviewID)
	}
	return item, nil
}

func buildFallbackQuestions(candidate CandidateSnapshot, jobTitle string) []RecommendedQuestion {
	jobTitle = strings.TrimSpace(jobTitle)
	if jobTitle == "" {
		jobTitle = "the role"
	}

	nameAnchor := candidate.FullName
	if nameAnchor == "" {
		nameAnchor = "the candidate"
	}
	skillAnchor := "core skills"
	if len(candidate.Skills) > 0 {
		skillAnchor = strings.Join(candidate.Skills, ", ")
	}

	return []RecommendedQuestion{
		{
			Category:  QuestionCategoryExperienceFollowUp,
			Question:  fmt.Sprintf("What project best represents %s's impact, and what measurable outcome was achieved?", nameAnchor),
			Focus:     "experience-impact",
			Reference: "template:experience",
		},
		{
			Category:  QuestionCategoryExperienceFollowUp,
			Question:  "Describe one failure in previous work and the corrective actions you personally led.",
			Focus:     "ownership-learning",
			Reference: "template:experience",
		},
		{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  fmt.Sprintf("For %s, which of your %s would you prioritize first in the first 90 days and why?", jobTitle, skillAnchor),
			Focus:     "capability-prioritization",
			Reference: "template:capability",
		},
		{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  "How do you validate quality and collaboration effectiveness before shipping a high-risk change?",
			Focus:     "quality-collaboration",
			Reference: "template:capability",
		},
	}
}

func ensureQuestionCoverage(in []RecommendedQuestion, candidate CandidateSnapshot, jobTitle string) []RecommendedQuestion {
	hasExperience := false
	hasCapability := false
	for _, item := range in {
		if item.Category == QuestionCategoryExperienceFollowUp {
			hasExperience = true
		}
		if item.Category == QuestionCategoryCapabilityAssessment {
			hasCapability = true
		}
	}

	out := append([]RecommendedQuestion(nil), in...)
	if !hasExperience {
		out = append(out, RecommendedQuestion{
			Category:  QuestionCategoryExperienceFollowUp,
			Question:  "Share a concrete project where your role changed the final decision outcome.",
			Focus:     "experience-depth",
			Reference: "coverage:experience",
		})
	}
	if !hasCapability {
		title := strings.TrimSpace(jobTitle)
		if title == "" {
			title = "this role"
		}
		out = append(out, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  fmt.Sprintf("Which capability is most critical for %s, and how would you demonstrate it in a practical case?", title),
			Focus:     "capability-fit",
			Reference: "coverage:capability",
		})
	}

	if len(candidate.Skills) == 0 {
		out = append(out, RecommendedQuestion{
			Category:  QuestionCategoryCapabilityAssessment,
			Question:  "Without relying on keyword skills, how would you prove your learning velocity in a new domain?",
			Focus:     "learning-agility",
			Reference: "coverage:no-skills",
		})
	}
	return out
}

func trimQuestions(in []RecommendedQuestion, max int) []RecommendedQuestion {
	if max <= 0 || len(in) <= max {
		return in
	}
	return in[:max]
}

func matchSkills(skills []string, lowerJD string) []string {
	if len(skills) == 0 || lowerJD == "" {
		return nil
	}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		if strings.Contains(lowerJD, strings.ToLower(skill)) {
			out = append(out, skill)
		}
	}
	return dedupNonEmpty(out)
}

func formatExperience(months int) string {
	if months <= 0 {
		return "recent years"
	}
	years := months / 12
	rem := months % 12
	if years == 0 {
		return fmt.Sprintf("%d months", months)
	}
	if rem == 0 {
		return fmt.Sprintf("%d years", years)
	}
	return fmt.Sprintf("%d years %d months", years, rem)
}
