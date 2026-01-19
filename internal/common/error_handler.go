// Package common provides error handling utilities for BaSyx Go components.
// It includes structured error types, HTTP status code error constructors,
// error classification functions, and standardized error response generation
// for consistent API error handling across all BaSyx services.
//
//nolint:all
package common

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// ErrorHandler represents a structured error response with metadata.
// It provides standardized error information including message type,
// error text, error code, correlation ID for tracking, and timestamp.
//
// The struct is JSON-serializable and follows BaSyx error response format
// for consistent API error reporting across all services.
type ErrorHandler struct {
	MessageType   string `json:"messageType"`             // Type of the message (e.g., "Error", "Warning")
	Text          string `json:"text"`                    // Human-readable error description
	Code          string `json:"code,omitempty"`          // HTTP status code as string
	CorrelationID string `json:"correlationId,omitempty"` // Unique identifier for error tracking
	Timestamp     string `json:"timestamp,omitempty"`     // RFC3339 formatted timestamp
}

// NewErrorHandler creates a new ErrorHandler instance with the provided parameters.
//
// Parameters:
//   - messageType: The type of message (typically "Error" for error responses)
//   - text: The error that occurred, will be converted to string using Error() method
//   - code: HTTP status code as string (e.g., "404", "500")
//   - correlationID: Unique identifier for tracking this error across systems
//   - timestamp: RFC3339 formatted timestamp when the error occurred
//
// Returns:
//   - *ErrorHandler: A pointer to the newly created ErrorHandler instance
//
// Example:
//
//	err := errors.New("resource not found")
//	handler := NewErrorHandler("Error", err, "404", "req-123", "2023-11-03T10:00:00Z")
func NewErrorHandler(messageType string, text error, code string, correlationID string, timestamp string) *ErrorHandler {
	return &ErrorHandler{
		MessageType:   messageType,
		Text:          text.Error(),
		Code:          code,
		CorrelationID: correlationID,
		Timestamp:     timestamp,
	}
}

// NewErrNotFound creates a standardized "404 Not Found" error.
//
// Parameters:
//   - elementID: The identifier of the element that was not found
//
// Returns:
//   - error: An error with message format "404 Not Found: <elementID>"
//
// Example:
//
//	err := NewErrNotFound("submodel-123")
//	// Returns error: "404 Not Found: submodel-123"
func NewErrNotFound(elementID string) error {
	return errors.New("404 Not Found: " + elementID)
}

// NewErrBadRequest creates a standardized "400 Bad Request" error.
//
// Parameters:
//   - message: Description of what made the request invalid
//
// Returns:
//   - error: An error with message format "400 Bad Request: <message>"
//
// Example:
//
//	err := NewErrBadRequest("invalid JSON format")
//	// Returns error: "400 Bad Request: invalid JSON format"
func NewErrBadRequest(message string) error {
	return errors.New("400 Bad Request: " + message)
}

// NewInternalServerError creates a standardized "500 Internal Server Error" error.
//
// Parameters:
//   - message: Description of the internal server error
//
// Returns:
//   - error: An error with message format "500 Internal Server Error: <message>"
//
// Example:
//
//	err := NewInternalServerError("database connection failed")
//	// Returns error: "500 Internal Server Error: database connection failed"
func NewInternalServerError(message string) error {
	return errors.New("500 Internal Server Error: " + message)
}

// NewErrConflict creates a standardized "409 Conflict" error.
//
// Parameters:
//   - message: Description of the conflict that occurred
//
// Returns:
//   - error: An error with message format "409 Conflict: <message>"
//
// Example:
//
//	err := NewErrConflict("resource already exists")
//	// Returns error: "409 Conflict: resource already exists"
func NewErrConflict(message string) error {
	return errors.New("409 Conflict: " + message)
}

// NewErrDenied creates a standardized "403 Denied" error.
//
// Parameters:
//   - message: Description of the denied action that occurred
//
// Returns:
//   - error: An error with message format "403 Denied: <message>"
func NewErrDenied(message string) error {
	return errors.New("403 Denied: " + message)
}

// IsErrNotFound checks if the given error is a "404 Not Found" error.
//
// Parameters:
//   - err: The error to check
//
// Returns:
//   - bool: true if the error is a 404 Not Found error, false otherwise
//
// Example:
//
//	err := NewErrNotFound("item-123")
//	if IsErrNotFound(err) {
//	    // Handle not found case
//	}
func IsErrNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "404 Not Found: ")
}

// IsErrBadRequest checks if the given error is a "400 Bad Request" error.
//
// Parameters:
//   - err: The error to check
//
// Returns:
//   - bool: true if the error is a 400 Bad Request error, false otherwise
//
// Example:
//
//	err := NewErrBadRequest("invalid input")
//	if IsErrBadRequest(err) {
//	    // Handle bad request case
//	}
func IsErrBadRequest(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "400 Bad Request: ")
}

// IsInternalServerError checks if the given error is a "500 Internal Server Error" error.
//
// Parameters:
//   - err: The error to check
//
// Returns:
//   - bool: true if the error is a 500 Internal Server Error, false otherwise
//
// Example:
//
//	err := NewInternalServerError("database error")
//	if IsInternalServerError(err) {
//	    // Handle internal server error case
//	}
func IsInternalServerError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "500 Internal Server Error: ")
}

// IsErrConflict checks if the given error is a "409 Conflict" error.
//
// Parameters:
//   - err: The error to check
//
// Returns:
//   - bool: true if the error is a 409 Conflict error, false otherwise
//
// Example:
//
//	err := NewErrConflict("duplicate entry")
//	if IsErrConflict(err) {
//	    // Handle conflict case
//	}
func IsErrConflict(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "409 Conflict: ")
}

// IsErrDenied checks if the given error is a "403 Denied" error.
//
// Parameters:
//   - err: The error to check
//
// Returns:
//   - bool: true if the error is a 403 Denied error, false otherwise
func IsErrDenied(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "403 Denied: ")
}

// NewErrorResponse creates a standardized HTTP error response with structured error information.
//
// This function generates a comprehensive error response that includes:
// - HTTP status code
// - Structured error handler with metadata
// - Internal correlation code for tracking
// - Current timestamp
//
// Parameters:
//   - err: The original error that occurred
//   - errorCode: HTTP status code (e.g., 404, 400, 500)
//   - component: Name of the component where the error occurred (e.g., "submodel", "registry")
//   - function: Name of the function where the error occurred
//   - info: Additional context information about the error
//
// Returns:
//   - model.ImplResponse: Standardized response object with error details
//
// The internal correlation code format is: "<component>-<code>-<function>-<statusText>-<info>"
//
// Example:
//
//	err := errors.New("submodel not found")
//	response := NewErrorResponse(err, 404, "submodel", "GetSubmodel", "invalidID")
//	// Creates response with correlation ID: "submodel-404-GetSubmodel-NotFound-invalidID"
func NewErrorResponse(err error, errorCode int, component string, function string, info string) model.ImplResponse {
	codeStr := strconv.Itoa(errorCode)
	statusText := strings.ReplaceAll(http.StatusText(errorCode), " ", "")
	internalCode := fmt.Sprintf("%s-%s-%s-%s-%s", component, codeStr, function, statusText, info)

	return model.Response(
		errorCode,
		[]ErrorHandler{
			*NewErrorHandler("Error", err, codeStr, internalCode, string(GetCurrentTimestamp())),
		},
	)
}

// NewAccessDeniedResponse returns a standardized HTTP 403 Forbidden error response.
//
// This function is used when a request is not allowed to proceed due to missing
// or invalid permissions. To avoid leaking internal implementation details,
// the response is intentionally generic and does not expose information about
// where or why the access decision was made.
//
// All access-denied situations intentionally produce the exact same structure,
// making it harder for callers to infer internal logic, rule configurations,
// or authorization paths.
//
// Returns:
//   - model.ImplResponse: A standardized 403 Forbidden response with a fixed
//     error message and correlation code pattern.
//
// Example:
//
//	response := NewAccessDeniedResponse()
//	// Produces a consistent "access denied" error without internal metadata.
func NewAccessDeniedResponse() model.ImplResponse {
	return NewErrorResponse(
		errors.New("access denied"), http.StatusForbidden, "Middleware", "Rules", "Denied",
	)
}
