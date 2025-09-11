package common

type ErrorHandler struct {
	MessageType   string `json:"messageType"`
	Text          string `json:"text"`
	Code          string `json:"code,omitempty"`
	CorrelationId string `json:"correlationId,omitempty"`
	Timestamp     string `json:"timestamp,omitempty"`
}
