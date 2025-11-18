package registryofregistriesapi

import (
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// DescriptionAPIAPIController binds http requests to an api service and writes the service results to the http response
type DescriptionAPIAPIController struct {
	service      DescriptionAPIAPIServicer
	errorHandler model.ErrorHandler
}

// DescriptionAPIAPIOption for how the controller is set up.
type DescriptionAPIAPIOption func(*DescriptionAPIAPIController)

// WithDescriptionAPIAPIErrorHandler inject ErrorHandler into controller
func WithDescriptionAPIAPIErrorHandler(h model.ErrorHandler) DescriptionAPIAPIOption {
	return func(c *DescriptionAPIAPIController) {
		c.errorHandler = h
	}
}

// NewDescriptionAPIAPIController creates a default api controller
func NewDescriptionAPIAPIController(s DescriptionAPIAPIServicer, opts ...DescriptionAPIAPIOption) *DescriptionAPIAPIController {
	controller := &DescriptionAPIAPIController{
		service:      s,
		errorHandler: model.DefaultErrorHandler,
	}

	for _, opt := range opts {
		opt(controller)
	}

	return controller
}

// Routes returns all the api routes for the DescriptionAPIAPIController
func (c *DescriptionAPIAPIController) Routes() Routes {
	return Routes{
		"GetDescription": Route{
			strings.ToUpper("Get"),
			"/description",
			c.GetDescription,
		},
	}
}

// GetDescription - Returns the self-describing information of a network resource (ServiceDescription)
func (c *DescriptionAPIAPIController) GetDescription(w http.ResponseWriter, r *http.Request) {
	result, err := c.service.GetDescription(r.Context())
	// If an error occurred, encode the error with the status code
	if err != nil {
		c.errorHandler(w, r, err, &result)
		return
	}
	// If no error, encode the body and the result code
	_ = EncodeJSONResponse(result.Body, &result.Code, w)
}
