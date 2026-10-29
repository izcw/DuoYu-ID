package main

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the single v1 business response contract.
// OAuth/OIDC and OpenAPI documents retain their standard wire formats.
type APIResponse struct {
	Code       int       `json:"code"`
	Msg        string    `json:"msg"`
	Data       any       `json:"data"`
	Error      *APIError `json:"error"`
	RequestID  string    `json:"request_id"`
	APIVersion string    `json:"api_version"`
}
type APIError struct {
	Code      string `json:"code"`
	Retryable bool   `json:"retryable"`
}

func apiErrorCode(status int) string {
	switch status {
	case 400:
		return "invalid_request"
	case 401:
		return "unauthenticated"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 405:
		return "method_not_allowed"
	case 409:
		return "conflict"
	case 412:
		return "precondition_failed"
	case 413:
		return "payload_too_large"
	case 422:
		return "validation_failed"
	case 429:
		return "rate_limited"
	case 503:
		return "service_unavailable"
	default:
		return "request_failed"
	}
}
func writeAPIResponse(w http.ResponseWriter, status int, msg string, data any, apiError *APIError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Duoyu-API-Version", "1")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{Code: status, Msg: msg, Data: data, Error: apiError, RequestID: w.Header().Get("X-Request-ID"), APIVersion: "1"})
}
func ok(w http.ResponseWriter, v any) { writeAPIResponse(w, http.StatusOK, "成功", v, nil) }
func fail(w http.ResponseWriter, status int, msg string) {
	writeAPIResponse(w, status, msg, nil, &APIError{Code: apiErrorCode(status), Retryable: status == 429 || status >= 500})
}
