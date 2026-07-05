package exigoapi

import (
	"encoding/json"
	"strings"
)

// Result is the decoded JSON body of a completed REST response: its fields
// as a generic tree, plus any business-level errors the operation reported
// via its result object.
type Result struct {
	Fields map[string]any
	Errors []string
}

// parseResponse decodes a REST response. Non-2xx statuses become
// *HTTPError (with *UnavailableError for retryable statuses that survived
// the retry loop); a 2xx body decodes into a *Result, itself carrying a
// *BusinessError when the result object reports errors.
func parseResponse(operation string, statusCode int, body []byte) (*Result, error) {
	if statusCode < 200 || statusCode > 299 {
		if isRetryableStatus(statusCode) {
			return nil, &UnavailableError{StatusCode: statusCode}
		}
		return nil, &HTTPError{StatusCode: statusCode, Message: errorMessage(body)}
	}

	fields := map[string]any{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &fields); err != nil {
			return nil, &HTTPError{StatusCode: statusCode, Message: "response was not a JSON object"}
		}
	}

	result := &Result{Fields: fields, Errors: extractErrors(fields)}
	if len(result.Errors) > 0 {
		return result, &BusinessError{Operation: operation, Errors: result.Errors}
	}
	return result, nil
}

// errorMessage extracts a human-readable message from an error response
// body: a JSON object's message/error field when present, otherwise the
// trimmed body text.
func errorMessage(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"message", "error", "detail"} {
			if s, ok := payload[key].(string); ok && s != "" {
				return s
			}
		}
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// extractErrors collects business-level errors from a completed response.
// The docs never show a populated result envelope, so this defensively
// covers the shapes the API plausibly mirrors from its SOAP Errors[]
// convention: a top-level errors array, or a result object carrying an
// errors array or non-empty error/message strings.
func extractErrors(fields map[string]any) []string {
	if errs := errorStrings(fields["errors"]); len(errs) > 0 {
		return errs
	}
	result, ok := fields["result"].(map[string]any)
	if !ok {
		return nil
	}
	if errs := errorStrings(result["errors"]); len(errs) > 0 {
		return errs
	}
	for _, key := range []string{"error", "message"} {
		if s, ok := result[key].(string); ok && s != "" && !resultReportsSuccess(result) {
			return []string{s}
		}
	}
	return nil
}

// resultReportsSuccess reports whether a result object's status marks the
// call successful, so a benign informational message is not misread as a
// business error. Status 0 mirrors the SOAP convention's success value.
func resultReportsSuccess(result map[string]any) bool {
	switch status := result["status"].(type) {
	case float64:
		return status == 0
	case string:
		return strings.EqualFold(status, "success") || status == "0"
	}
	return false
}

func errorStrings(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
