package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zplzpl/resume_system/internal/report"
)

func TestGenerateAndExportFlow(t *testing.T) {
	svc := report.NewService(func() time.Time {
		return time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	})
	server := NewServer(svc)

	body := map[string]interface{}{
		"candidate": map[string]interface{}{
			"id":       "cand-1",
			"name":     "Alice",
			"position": "Product Manager",
		},
		"evaluations": []map[string]interface{}{
			{
				"interview_id":     "round-1",
				"interviewer_id":   "u-1",
				"interviewer_name": "Bob",
				"overall_score":    4.3,
				"summary":          "Strong ownership.",
				"dimensions": []map[string]interface{}{
					{
						"name":    "Ownership",
						"score":   4.4,
						"comment": "Drives decisions.",
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, body=%s", w.Code, w.Body.String())
	}

	var generated report.StructuredInterviewReport
	if err := json.Unmarshal(w.Body.Bytes(), &generated); err != nil {
		t.Fatalf("failed to decode generate response: %v", err)
	}
	if generated.ReportID == "" {
		t.Fatalf("expected report id")
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/reports/"+generated.ReportID+"/export?format=markdown", nil)
	exportW := httptest.NewRecorder()
	server.Handler().ServeHTTP(exportW, exportReq)
	if exportW.Code != http.StatusOK {
		t.Fatalf("export status = %d, body=%s", exportW.Code, exportW.Body.String())
	}
	if !strings.Contains(exportW.Body.String(), "Hiring Recommendation") {
		t.Fatalf("expected markdown export body")
	}

	xlsxReq := httptest.NewRequest(http.MethodGet, "/api/reports/"+generated.ReportID+"/export?format=xlsx", nil)
	xlsxW := httptest.NewRecorder()
	server.Handler().ServeHTTP(xlsxW, xlsxReq)
	if xlsxW.Code != http.StatusOK {
		t.Fatalf("xlsx export status = %d, body=%s", xlsxW.Code, xlsxW.Body.String())
	}
	if !strings.Contains(xlsxW.Header().Get("Content-Type"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
		t.Fatalf("expected xlsx content type, got %s", xlsxW.Header().Get("Content-Type"))
	}
	reader, err := zip.NewReader(bytes.NewReader(xlsxW.Body.Bytes()), int64(xlsxW.Body.Len()))
	if err != nil {
		t.Fatalf("failed to parse xlsx zip: %v", err)
	}
	var sheetXML string
	for _, f := range reader.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			t.Fatalf("failed to open worksheet xml: %v", openErr)
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatalf("failed to read worksheet xml: %v", readErr)
		}
		sheetXML = string(data)
		break
	}
	if !strings.Contains(sheetXML, "candidate_name") {
		t.Fatalf("expected xlsx export header row")
	}
}

func TestGenerateInvalidJSON(t *testing.T) {
	svc := report.NewService(nil)
	server := NewServer(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/reports/generate", strings.NewReader("{"))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "INVALID_JSON") {
		t.Fatalf("expected INVALID_JSON, got %s", w.Body.String())
	}
}

func TestExportUnknownReport(t *testing.T) {
	svc := report.NewService(nil)
	server := NewServer(svc)

	req := httptest.NewRequest(http.MethodGet, "/api/reports/not-found/export?format=json", nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "REPORT_NOT_FOUND") {
		t.Fatalf("expected REPORT_NOT_FOUND, got %s", w.Body.String())
	}
}
