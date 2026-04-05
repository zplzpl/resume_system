package audit

type Service struct {
	repo *MemoryRepository
}

func NewService(repo *MemoryRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Record(input RecordInput) Event {
	return s.repo.Add(input)
}

func (s *Service) Query(filter QueryFilter) []Event {
	return s.repo.Query(filter)
}
