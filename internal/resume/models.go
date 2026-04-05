package resume

import "time"

type ParseStatus string

const (
	ParseStatusPending    ParseStatus = "pending"
	ParseStatusProcessing ParseStatus = "processing"
	ParseStatusSuccess    ParseStatus = "success"
	ParseStatusFailed     ParseStatus = "failed"
)

type ParsedCoreFields struct {
	FullName              string   `json:"full_name"`
	Email                 string   `json:"email,omitempty"`
	Phone                 string   `json:"phone,omitempty"`
	CurrentCompany        string   `json:"current_company,omitempty"`
	CurrentTitle          string   `json:"current_title,omitempty"`
	Location              string   `json:"location,omitempty"`
	HighestEducation      string   `json:"highest_education,omitempty"`
	TotalExperienceMonths int      `json:"total_experience_months,omitempty"`
	Skills                []string `json:"skills,omitempty"`
}

func (p ParsedCoreFields) IsEmpty() bool {
	return p.FullName == "" && p.Email == "" && p.Phone == ""
}

type ResumeRecord struct {
	ID            string           `json:"id"`
	CandidateID   string           `json:"candidate_id,omitempty"`
	FileName      string           `json:"file_name"`
	ContentType   string           `json:"content_type"`
	FileSize      int64            `json:"file_size"`
	FileHash      string           `json:"file_hash"`
	StoragePath   string           `json:"storage_path"`
	ParseStatus   ParseStatus      `json:"parse_status"`
	FailureReason string           `json:"failure_reason,omitempty"`
	ParsedPayload ParsedCoreFields `json:"parsed_payload,omitempty"`
	UploadedBy    string           `json:"uploaded_by,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	ParsedAt      *time.Time       `json:"parsed_at,omitempty"`
}

type CandidateProfile struct {
	ID                    string    `json:"id"`
	FullName              string    `json:"full_name"`
	Email                 string    `json:"email,omitempty"`
	Phone                 string    `json:"phone,omitempty"`
	CurrentCompany        string    `json:"current_company,omitempty"`
	CurrentTitle          string    `json:"current_title,omitempty"`
	Location              string    `json:"location,omitempty"`
	HighestEducation      string    `json:"highest_education,omitempty"`
	TotalExperienceMonths int       `json:"total_experience_months,omitempty"`
	Skills                []string  `json:"skills,omitempty"`
	SourceResumeID        string    `json:"source_resume_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// UploadFile contains one uploaded file from the API layer.
type UploadFile struct {
	FileName    string
	ContentType string
	Reader      FileReader
}

type FileReader interface {
	Read(p []byte) (n int, err error)
}
