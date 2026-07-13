/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.2.0
 * Contact: info@idtwin.org
 */

package openapi

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

var (
	// ErrTypeAssertionError is thrown when type an interface does not match the asserted type
	ErrTypeAssertionError = errors.New("unable to assert type")
)

// ParsingError indicates that an error has occurred when parsing request parameters
type ParsingError struct {
	Param string
	Err   error
}

func (e *ParsingError) Unwrap() error {
	return e.Err
}

func (e *ParsingError) Error() string {
	if e.Param == "" {
		return e.Err.Error()
	}

	return e.Param + ": " + e.Err.Error()
}

// RequiredError indicates that an error has occurred when parsing request parameters
type RequiredError struct {
	Field string
}

func (e *RequiredError) Error() string {
	return fmt.Sprintf("required field '%s' is zero value.", e.Field)
}

// ErrorHandler defines the required method for handling error. You may implement it and inject this into a controller if
// you would like errors to be handled differently from the DefaultErrorHandler
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error, result *model.ImplResponse)

// DefaultErrorHandler defines the default logic on how to handle errors from the controller. Any errors from parsing
// request params will return a StatusBadRequest. Otherwise, the error code originating from the servicer will be used.
func DefaultErrorHandler(w http.ResponseWriter, _ *http.Request, err error, result *model.ImplResponse) {
	status := http.StatusInternalServerError
	info := "Service"

	var parsingErr *ParsingError
	if ok := errors.As(err, &parsingErr); ok {
		status = http.StatusBadRequest
		info = "ParseRequest"
		var maxBytesErr *http.MaxBytesError
		if errors.As(parsingErr.Err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
			info = "RequestBodyTooLarge"
		}
	} else {
		var requiredErr *RequiredError
		var modelRequiredErr *model.RequiredError
		if errors.As(err, &requiredErr) || errors.As(err, &modelRequiredErr) {
			status = http.StatusUnprocessableEntity
			info = "RequiredParameter"
		} else if result != nil && result.Code != 0 {
			status = result.Code
		}
	}

	_ = model.WriteErrorResponse(w, err, status, "SMREPO", "DefaultErrorHandler", info)
}
