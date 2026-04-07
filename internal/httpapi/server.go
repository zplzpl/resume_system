package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/zplzpl/resume_system/internal/report"
)

type Server struct {
	service *report.Service
	mux     *http.ServeMux
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func NewServer(service *report.Service) *Server {
	s := &Server{
		service: service,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/reports/generate", s.handleGenerateReport)
	s.mux.HandleFunc("/api/reports/", s.handleExportReport)
}

func (s *Server) handleGenerateReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only POST is supported", nil)
		return
	}

	defer r.Body.Close()
	var req report.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "request body must be valid JSON", map[string]interface{}{
			"reason": err.Error(),
		})
		return
	}

	generated, err := s.service.Generate(r.Context(), req)
	if err != nil {
		var verr report.ValidationErrors
		if errors.As(err, &verr) {
			writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "report generation request is invalid", map[string]interface{}{
				"fields": verr,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate report", map[string]interface{}{
			"reason": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, generated)
}

func (s *Server) handleExportReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "only GET is supported", nil)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/reports/")
	if !strings.HasSuffix(path, "/export") {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "endpoint not found", nil)
		return
	}

	reportID := strings.TrimSuffix(path, "/export")
	reportID = strings.TrimSuffix(reportID, "/")
	reportID = strings.TrimSpace(reportID)
	if reportID == "" {
		writeError(w, http.StatusBadRequest, "REPORT_ID_REQUIRED", "report id is required", nil)
		return
	}

	format := r.URL.Query().Get("format")
	fileName, contentType, content, err := s.service.Export(reportID, format)
	if err != nil {
		if errors.Is(err, report.ErrReportNotFound) {
			writeError(w, http.StatusNotFound, "REPORT_NOT_FOUND", "report does not exist", map[string]interface{}{
				"report_id": reportID,
			})
			return
		}
		if errors.Is(err, report.ErrUnsupportedExportFormat) {
			writeError(w, http.StatusBadRequest, "UNSUPPORTED_EXPORT_FORMAT", "format must be one of: json, markdown, md, xlsx", map[string]interface{}{
				"format": format,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to export report", map[string]interface{}{
			"reason": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string, details map[string]interface{}) {
	writeJSON(w, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
