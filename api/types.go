package api

type SearchResponse struct {
	Items    []map[string]interface{} `json:"items"`
	NextLink string                   `json:"nextLink,omitempty"`
}

// Based on https://github.com/microsoft/api-guidelines/blob/vNext/Guidelines.md#7102-error-condition-responses
type ErrorResponse struct {
	Error ErrorInfo `json:"error"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func CreateErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{Error: ErrorInfo{Code: code, Message: message}}
}
