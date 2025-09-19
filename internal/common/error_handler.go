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

func NewErrNotFound(elementId string) error {
	return errors.New("404 Not Found: " + elementId)
}

func IsErrNotFound(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "404 Not Found: ")
}
