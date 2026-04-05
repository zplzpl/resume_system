package resume

import "testing"

func TestParseNaturalCandidateQueryCombinedConditions(t *testing.T) {
	plan, err := ParseNaturalCandidateQuery("筛选中，技能: Go, SQL，公司: ACME，学校: Tsinghua，地点: 北京，3年以上")
	if err != nil {
		t.Fatalf("expected parse success, got err: %v", err)
	}
	if len(plan.Options.StatusList) != 1 || plan.Options.StatusList[0] != CandidateStatusScreening {
		t.Fatalf("expected screening status, got %+v", plan.Options.StatusList)
	}
	if len(plan.Options.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %+v", plan.Options.Skills)
	}
	if plan.Options.Company != "ACME" {
		t.Fatalf("expected company ACME, got %q", plan.Options.Company)
	}
	if plan.Options.School != "Tsinghua" {
		t.Fatalf("expected school Tsinghua, got %q", plan.Options.School)
	}
	if plan.Options.Location != "北京" {
		t.Fatalf("expected location 北京, got %q", plan.Options.Location)
	}
	if plan.Options.MinExperienceMonths != 36 {
		t.Fatalf("expected min experience 36 months, got %d", plan.Options.MinExperienceMonths)
	}
	if len(plan.Conditions) < 6 {
		t.Fatalf("expected multiple parsed conditions, got %+v", plan.Conditions)
	}
}

func TestParseNaturalCandidateQueryFallbackKeyword(t *testing.T) {
	plan, err := ParseNaturalCandidateQuery("golang backend")
	if err != nil {
		t.Fatalf("expected parse success, got err: %v", err)
	}
	if plan.Options.Keyword == "" {
		t.Fatalf("expected fallback keyword to be populated")
	}
}

func TestParseNaturalCandidateQueryFailure(t *testing.T) {
	_, err := ParseNaturalCandidateQuery("!!! ???")
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !IsNaturalQueryErr(err) {
		t.Fatalf("expected natural query error, got %v", err)
	}
}
