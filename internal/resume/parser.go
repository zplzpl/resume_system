package resume

import (
	"archive/zip"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported file type, only PDF/DOC/DOCX are allowed")
	ErrCoreFieldsMissing = errors.New("unable to extract core resume fields")
)

var supportedExtensions = map[string]struct{}{
	".pdf":  {},
	".doc":  {},
	".docx": {},
}

type HeuristicParser struct{}

func NewHeuristicParser() *HeuristicParser {
	return &HeuristicParser{}
}

func IsSupportedFileName(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	_, ok := supportedExtensions[ext]
	return ok
}

func (p *HeuristicParser) Parse(storagePath string) (ParsedCoreFields, error) {
	ext := strings.ToLower(filepath.Ext(storagePath))
	if _, ok := supportedExtensions[ext]; !ok {
		return ParsedCoreFields{}, ErrUnsupportedFormat
	}

	var raw string
	var err error
	switch ext {
	case ".docx":
		raw, err = extractDOCXText(storagePath)
	default:
		raw, err = extractBinaryText(storagePath)
	}
	if err != nil {
		return ParsedCoreFields{}, err
	}

	parsed := mapCoreFields(normalizeText(raw))
	if parsed.IsEmpty() {
		return ParsedCoreFields{}, ErrCoreFieldsMissing
	}
	return parsed, nil
}

func extractDOCXText(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	defer reader.Close()

	var parts []string
	for _, f := range reader.File {
		if !strings.HasPrefix(f.Name, "word/") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b, readErr := io.ReadAll(rc)
		rc.Close()
		if readErr != nil {
			continue
		}
		parts = append(parts, stripXML(string(b)))
	}
	if len(parts) == 0 {
		return "", errors.New("docx has no readable text segments")
	}
	return strings.Join(parts, "\n"), nil
}

func extractBinaryText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	if len(b) == 0 {
		return "", errors.New("empty file")
	}

	buf := make([]rune, 0, len(b))
	for _, v := range b {
		switch {
		case v == '\n' || v == '\r' || v == '\t':
			buf = append(buf, rune(v))
		case v >= 32 && v <= 126:
			buf = append(buf, rune(v))
		default:
			buf = append(buf, ' ')
		}
	}
	return string(buf), nil
}

func mapCoreFields(text string) ParsedCoreFields {
	parsed := ParsedCoreFields{}
	parsed.Email = matchFirst(emailRe, text)
	parsed.Phone = normalizePhone(matchFirst(phoneRe, text))

	parsed.FullName = extractLabeledValue(text, []string{"name", "candidate", "姓名"})
	if parsed.FullName == "" {
		parsed.FullName = extractNameFromTopLine(text)
	}

	parsed.CurrentCompany = extractLabeledValue(text, []string{"current company", "company", "公司"})
	parsed.CurrentTitle = extractLabeledValue(text, []string{"title", "position", "职位"})
	parsed.Location = extractLabeledValue(text, []string{"location", "city", "地区"})
	parsed.HighestEducation = extractLabeledValue(text, []string{"education", "degree", "学历"})
	parsed.TotalExperienceMonths = extractExperienceMonths(text)
	parsed.Skills = splitSkillList(extractLabeledValue(text, []string{"skills", "skill", "技能"}))

	if parsed.FullName == "" && parsed.Email != "" {
		parsed.FullName = deriveNameFromEmail(parsed.Email)
	}
	return parsed
}

func normalizeText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

var xmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripXML(s string) string {
	s = strings.ReplaceAll(s, "</w:p>", "\n")
	s = strings.ReplaceAll(s, "<w:br/>", "\n")
	s = xmlTagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return s
}

var (
	emailRe      = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	phoneRe      = regexp.MustCompile(`(?:\+?\d[\d\s\-()]{7,}\d)`)
	yearsENRe    = regexp.MustCompile(`(?i)\b(\d{1,2})\+?\s*(?:years?|yrs?)\b`)
	yearsZHRe    = regexp.MustCompile(`(\d{1,2})\s*年`)
	monthsENRe   = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(?:months?|mos?)\b`)
	monthsZHRe   = regexp.MustCompile(`(\d{1,3})\s*月`)
	nonWordRe    = regexp.MustCompile(`[^a-zA-Z\s\-]`)
	splitSkillRe = regexp.MustCompile(`[，,;/|、]+`)
)

func matchFirst(re *regexp.Regexp, s string) string {
	return strings.TrimSpace(re.FindString(s))
}

func normalizePhone(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, " ", "")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, "(", "")
	v = strings.ReplaceAll(v, ")", "")
	return v
}

func extractLabeledValue(text string, labels []string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		for _, label := range labels {
			if !strings.Contains(lower, strings.ToLower(label)) {
				continue
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 1 {
				parts = strings.SplitN(line, "：", 2)
			}
			if len(parts) == 2 {
				value := strings.TrimSpace(parts[1])
				if value != "" {
					return cleanExtractedValue(value)
				}
			}
		}
	}
	return ""
}

func cleanExtractedValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "|-•")
	if len(v) > 128 {
		v = v[:128]
	}
	return strings.TrimSpace(v)
}

func extractNameFromTopLine(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, "@") || strings.Contains(line, ":") {
			continue
		}
		if len(strings.Fields(line)) > 6 || len(line) > 48 {
			continue
		}
		if nonWordRe.ReplaceAllString(line, "") == "" {
			continue
		}
		return line
	}
	return ""
}

func extractExperienceMonths(text string) int {
	if m := yearsENRe.FindStringSubmatch(text); len(m) == 2 {
		if years, err := strconv.Atoi(m[1]); err == nil {
			return years * 12
		}
	}
	if m := yearsZHRe.FindStringSubmatch(text); len(m) == 2 {
		if years, err := strconv.Atoi(m[1]); err == nil {
			return years * 12
		}
	}
	if m := monthsENRe.FindStringSubmatch(text); len(m) == 2 {
		if months, err := strconv.Atoi(m[1]); err == nil {
			return months
		}
	}
	if m := monthsZHRe.FindStringSubmatch(text); len(m) == 2 {
		if months, err := strconv.Atoi(m[1]); err == nil {
			return months
		}
	}
	return 0
}

func splitSkillList(v string) []string {
	if v == "" {
		return nil
	}
	parts := splitSkillRe.Split(v, -1)
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deriveNameFromEmail(email string) string {
	local, _, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return ""
	}
	local = strings.ReplaceAll(local, ".", " ")
	local = strings.ReplaceAll(local, "_", " ")
	local = strings.ReplaceAll(local, "-", " ")
	words := strings.Fields(local)
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " ")
}
