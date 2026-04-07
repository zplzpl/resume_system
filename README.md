# resume_system

Structured interview report generation service for the Resume System MVP.

## Run

```bash
go run ./cmd/server
```

Server starts at `:8080` by default. Override with `PORT`.

## API

### Generate report

`POST /api/reports/generate`

Request example:

```json
{
  "candidate": {
    "id": "cand-1001",
    "name": "Jane Doe",
    "email": "jane@example.com",
    "phone": "+1-555-0100",
    "position": "Backend Engineer"
  },
  "evaluations": [
    {
      "interview_id": "round-1",
      "interviewer_id": "u-hr-1",
      "interviewer_name": "HR",
      "overall_score": 4.2,
      "summary": "Communication is clear and proactive.",
      "dimensions": [
        { "name": "Communication", "score": 4.5, "comment": "Strong articulation." },
        { "name": "Problem Solving", "score": 4.0, "comment": "Good decomposition." }
      ]
    }
  ],
  "generated_by": "hr-user-1"
}
```

### Export report

`GET /api/reports/{reportID}/export?format=json|markdown|csv`

Supported formats:
- `json`
- `markdown` (or `md`)
- `csv`

## Test

```bash
go test ./...
```
