package common

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type ErrorHandler struct {
	MessageType   string `json:"messageType"`
	Text          string `json:"text"`
	Code          string `json:"code,omitempty"`
	CorrelationId string `json:"correlationId,omitempty"`
	Timestamp     string `json:"timestamp,omitempty"`
}

func NewErrorHandler(messageType string, text error, code string, correlationId string, timestamp string) *ErrorHandler {
	return &ErrorHandler{
		MessageType:   messageType,
		Text:          text.Error(),
		Code:          code,
		CorrelationId: correlationId,
		Timestamp:     timestamp,
	}
}

func NewErrNotFound(elementId string) error {
	return errors.New("404 Not Found: " + elementId)
}

func NewErrBadRequest(message string) error {
	return errors.New("400 Bad Request: " + message)
}

func NewInternalServerError(message string) error {
	return errors.New("500 Internal Server Error: " + message)
}

func NewErrConflict(message string) error {
	return errors.New("409 Conflict: " + message)
}

func IsErrNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "404 Not Found: ")
}

func IsErrBadRequest(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "400 Bad Request: ")
}

func IsInternalServerError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "500 Internal Server Error: ")
}

func IsErrConflict(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "409 Conflict: ")
}

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
