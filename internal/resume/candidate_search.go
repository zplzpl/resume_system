package resume

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrCandidateNotFound  = errors.New("candidate not found")
	ErrResumeNotFound     = errors.New("resume not found")
	ErrInvalidStatusLayer = errors.New("invalid status layer")
	ErrNaturalQuery       = errors.New("unable to parse natural language query")
)

type CandidateSearchOptions struct {
	Keyword             string
	Skill               string
	Skills              []string
	Company             string
	School              string
	Location            string
	StatusList          []CandidateStatusLayer
	MinExperienceMonths int
	MaxExperienceMonths int
}

type ParsedCandidateCondition struct {
	Field       string `json:"field"`
	Operator    string `json:"operator"`
	Value       any    `json:"value"`
	Description string `json:"description"`
}

type NaturalCandidateQueryPlan struct {
	Options    CandidateSearchOptions `json:"options"`
	Conditions []ParsedCandidateCondition
}

func ParseCandidateStatusLayer(raw string) (CandidateStatusLayer, error) {
	switch CandidateStatusLayer(strings.ToLower(strings.TrimSpace(raw))) {
	case CandidateStatusNew:
		return CandidateStatusNew, nil
	case CandidateStatusScreening:
		return CandidateStatusScreening, nil
	case CandidateStatusInterview:
		return CandidateStatusInterview, nil
	case CandidateStatusOffer:
		return CandidateStatusOffer, nil
	case CandidateStatusHired:
		return CandidateStatusHired, nil
	case CandidateStatusArchived:
		return CandidateStatusArchived, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidStatusLayer, raw)
	}
}

func ParseCandidateStatusLayers(raw []string) ([]CandidateStatusLayer, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	out := make([]CandidateStatusLayer, 0, len(raw))
	seen := make(map[CandidateStatusLayer]struct{}, len(raw))
	for _, item := range raw {
		for _, part := range strings.Split(item, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			status, err := ParseCandidateStatusLayer(value)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[status]; ok {
				continue
			}
			seen[status] = struct{}{}
			out = append(out, status)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

var (
	splitNaturalValueRe = regexp.MustCompile(`[，,;/|、]+`)
	experienceRangeRe   = regexp.MustCompile(`(?i)(\d{1,2})\s*(?:-|~|到|至)\s*(\d{1,2})\s*(?:年|years?|yrs?)`)
	experienceMinRe     = regexp.MustCompile(`(?i)(?:至少|不少于|minimum|min|>=)?\s*(\d{1,2})\s*(?:\+|以上|及以上)?\s*(?:年|years?|yrs?)`)
	experienceMaxRe     = regexp.MustCompile(`(?i)(?:不超过|最多|至多|maximum|max|<=)\s*(\d{1,2})\s*(?:年|years?|yrs?)`)
	nonTextRe           = regexp.MustCompile(`[^\p{Han}a-zA-Z0-9\s]`)
	multiSpaceRe        = regexp.MustCompile(`\s+`)
)

func ParseNaturalCandidateQuery(raw string) (NaturalCandidateQueryPlan, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return NaturalCandidateQueryPlan{}, fmt.Errorf("%w: empty query", ErrNaturalQuery)
	}

	plan := NaturalCandidateQueryPlan{}
	conditions := make([]ParsedCandidateCondition, 0, 8)
	consumed := query

	statuses, statusTerms := parseNaturalStatuses(query)
	if len(statuses) > 0 {
		plan.Options.StatusList = statuses
		values := make([]string, 0, len(statuses))
		for _, status := range statuses {
			values = append(values, string(status))
		}
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "status_layer",
			Operator:    "in",
			Value:       values,
			Description: fmt.Sprintf("status in %s", strings.Join(values, ", ")),
		})
		for _, term := range statusTerms {
			consumed = strings.ReplaceAll(consumed, term, " ")
		}
	}

	if skills, matched := extractNaturalListValue(query, []string{"skills", "skill", "技能", "擅长", "熟悉", "精通", "会"}); len(skills) > 0 {
		plan.Options.Skills = skills
		plan.Options.Skill = skills[0]
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "skills",
			Operator:    "contains_all",
			Value:       skills,
			Description: fmt.Sprintf("skills include %s", strings.Join(skills, ", ")),
		})
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	if company, matched := extractNaturalSingleValue(query, []string{"company", "公司", "来自", "就职于", "任职于"}); company != "" {
		plan.Options.Company = company
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "company",
			Operator:    "contains",
			Value:       company,
			Description: fmt.Sprintf("company contains %q", company),
		})
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	if school, matched := extractNaturalSingleValue(query, []string{"school", "学校", "毕业于", "毕业院校"}); school != "" {
		plan.Options.School = school
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "school",
			Operator:    "contains",
			Value:       school,
			Description: fmt.Sprintf("school contains %q", school),
		})
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	if location, matched := extractNaturalSingleValue(query, []string{"location", "城市", "地区", "地点"}); location != "" {
		plan.Options.Location = location
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "location",
			Operator:    "contains",
			Value:       location,
			Description: fmt.Sprintf("location contains %q", location),
		})
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	if keyword, matched := extractNaturalSingleValue(query, []string{"keyword", "关键词"}); keyword != "" {
		plan.Options.Keyword = keyword
		conditions = append(conditions, ParsedCandidateCondition{
			Field:       "keyword",
			Operator:    "contains",
			Value:       keyword,
			Description: fmt.Sprintf("keyword contains %q", keyword),
		})
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	if yearsMin, yearsMax, matched := parseNaturalExperience(query); yearsMin > 0 || yearsMax > 0 {
		if yearsMin > 0 {
			plan.Options.MinExperienceMonths = yearsMin * 12
			conditions = append(conditions, ParsedCandidateCondition{
				Field:       "experience_years",
				Operator:    ">=",
				Value:       yearsMin,
				Description: fmt.Sprintf("experience >= %d years", yearsMin),
			})
		}
		if yearsMax > 0 {
			plan.Options.MaxExperienceMonths = yearsMax * 12
			conditions = append(conditions, ParsedCandidateCondition{
				Field:       "experience_years",
				Operator:    "<=",
				Value:       yearsMax,
				Description: fmt.Sprintf("experience <= %d years", yearsMax),
			})
		}
		consumed = strings.ReplaceAll(consumed, matched, " ")
	}

	plan.Conditions = conditions

	if plan.Options.Keyword == "" {
		remainder := cleanNaturalRemainder(consumed)
		if remainder != "" {
			plan.Options.Keyword = remainder
			plan.Conditions = append(plan.Conditions, ParsedCandidateCondition{
				Field:       "keyword",
				Operator:    "contains",
				Value:       remainder,
				Description: fmt.Sprintf("keyword contains %q", remainder),
			})
		}
	}

	if len(plan.Conditions) == 0 {
		return NaturalCandidateQueryPlan{}, fmt.Errorf("%w: no recognizable conditions", ErrNaturalQuery)
	}
	return plan, nil
}

func IsNaturalQueryErr(err error) bool {
	return errors.Is(err, ErrNaturalQuery)
}

func parseNaturalStatuses(query string) ([]CandidateStatusLayer, []string) {
	type synonym struct {
		status CandidateStatusLayer
		terms  []string
	}
	synonyms := []synonym{
		{status: CandidateStatusScreening, terms: []string{"screening", "筛选中", "筛选", "初筛"}},
		{status: CandidateStatusInterview, terms: []string{"interview", "面试中", "面试"}},
		{status: CandidateStatusOffer, terms: []string{"offer", "待入职", "录用"}},
		{status: CandidateStatusHired, terms: []string{"hired", "已入职"}},
		{status: CandidateStatusArchived, terms: []string{"archived", "归档", "人才库"}},
		{status: CandidateStatusNew, terms: []string{"new", "新建", "待筛选"}},
	}

	lower := strings.ToLower(query)
	statuses := make([]CandidateStatusLayer, 0, 4)
	seen := map[CandidateStatusLayer]struct{}{}
	matchedTerms := make([]string, 0, 6)
	for _, item := range synonyms {
		for _, term := range item.terms {
			if term == "" {
				continue
			}
			checkTerm := term
			if term == strings.ToLower(term) {
				checkTerm = strings.ToLower(term)
			}
			if !strings.Contains(lower, strings.ToLower(checkTerm)) && !strings.Contains(query, term) {
				continue
			}
			if _, ok := seen[item.status]; !ok {
				seen[item.status] = struct{}{}
				statuses = append(statuses, item.status)
			}
			matchedTerms = append(matchedTerms, term)
		}
	}
	return statuses, matchedTerms
}

func extractNaturalSingleValue(query string, labels []string) (value string, matched string) {
	for _, label := range labels {
		pattern := fmt.Sprintf(`(?i)%s\s*(?:是|为|:|：|=)?\s*([^\n,，。；;]+)`, regexp.QuoteMeta(label))
		re := regexp.MustCompile(pattern)
		loc := re.FindStringSubmatchIndex(query)
		if len(loc) < 4 {
			continue
		}
		value = strings.TrimSpace(query[loc[2]:loc[3]])
		matched = strings.TrimSpace(query[loc[0]:loc[1]])
		value = cleanNaturalValue(value)
		if value != "" {
			return value, matched
		}
	}
	return "", ""
}

func extractNaturalListValue(query string, labels []string) (values []string, matched string) {
	raw := ""
	for _, label := range labels {
		pattern := fmt.Sprintf(`(?i)%s\s*(?:是|为|:|：|=)?\s*([^\n。；;]+)`, regexp.QuoteMeta(label))
		re := regexp.MustCompile(pattern)
		loc := re.FindStringSubmatchIndex(query)
		if len(loc) < 4 {
			continue
		}
		raw = strings.TrimSpace(query[loc[2]:loc[3]])
		matched = strings.TrimSpace(query[loc[0]:loc[1]])
		break
	}
	if raw == "" {
		return nil, ""
	}
	lowerRaw := strings.ToLower(raw)
	stopLabels := []string{"company", "公司", "school", "学校", "毕业于", "毕业院校", "location", "城市", "地区", "地点", "keyword", "关键词"}
	for _, stop := range stopLabels {
		idx := strings.Index(lowerRaw, strings.ToLower(stop))
		if idx <= 0 {
			continue
		}
		raw = strings.TrimSpace(raw[:idx])
		lowerRaw = strings.ToLower(raw)
	}

	parts := splitNaturalValueRe.Split(raw, -1)
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := cleanNaturalValue(part)
		if strings.Contains(item, ":") || strings.Contains(item, "：") {
			break
		}
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		values = append(values, item)
	}
	return values, matched
}

func parseNaturalExperience(query string) (minYears int, maxYears int, matched string) {
	if m := experienceRangeRe.FindStringSubmatch(query); len(m) == 3 {
		minYears, _ = strconv.Atoi(m[1])
		maxYears, _ = strconv.Atoi(m[2])
		matched = m[0]
		if minYears > maxYears {
			minYears, maxYears = maxYears, minYears
		}
		return minYears, maxYears, matched
	}
	if m := experienceMaxRe.FindStringSubmatch(query); len(m) == 2 {
		maxYears, _ = strconv.Atoi(m[1])
		matched = m[0]
	}
	if m := experienceMinRe.FindStringSubmatch(query); len(m) == 2 {
		minYears, _ = strconv.Atoi(m[1])
		if matched == "" {
			matched = m[0]
		}
	}
	return minYears, maxYears, matched
}

func cleanNaturalRemainder(value string) string {
	value = nonTextRe.ReplaceAllString(value, " ")
	value = strings.TrimSpace(multiSpaceRe.ReplaceAllString(value, " "))
	if value == "" {
		return ""
	}
	if len([]rune(value)) < 2 {
		return ""
	}
	return value
}

func cleanNaturalValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'[](){}<>`)
	if len(value) > 128 {
		value = value[:128]
	}
	return strings.TrimSpace(value)
}
