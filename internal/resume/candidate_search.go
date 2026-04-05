package resume

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCandidateNotFound  = errors.New("candidate not found")
	ErrResumeNotFound     = errors.New("resume not found")
	ErrInvalidStatusLayer = errors.New("invalid status layer")
)

type CandidateSearchOptions struct {
	Keyword    string
	Skill      string
	Company    string
	School     string
	StatusList []CandidateStatusLayer
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
