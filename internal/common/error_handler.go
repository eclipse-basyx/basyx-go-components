package common

import (
	"errors"
	"strings"
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

func IsErrNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "404 Not Found: ")
}

func IsErrBadRequest(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "400 Bad Request: ")
}
