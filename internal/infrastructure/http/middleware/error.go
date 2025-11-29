package middleware

import (
	"fmt"
	"net/http"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/labstack/echo/v4"
)

// ErrorHandler is a custom error handler for Echo
func ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		// Check if it's a domain error
		var domainErr *errors.DomainError
		if errors.As(err, &domainErr) {
			statusCode := mapDomainErrorToHTTPStatus(domainErr)

			c.JSON(statusCode, dto.ErrorResponse{
				Error:   domainErr.Code,
				Message: domainErr.Message,
				Code:    domainErr.Code,
			})
			return
		}

		// Check if it's an Echo HTTP error
		if he, ok := err.(*echo.HTTPError); ok {
			c.JSON(he.Code, dto.ErrorResponse{
				Error:   http.StatusText(he.Code),
				Message: fmt.Sprintf("%v", he.Message),
			})
			return
		}

		// Default to internal server error
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:   "internal_error",
			Message: err.Error(),
		})
	}
}

// mapDomainErrorToHTTPStatus maps domain errors to HTTP status codes
func mapDomainErrorToHTTPStatus(err *errors.DomainError) int {
	switch err.Code {
	case "NOT_FOUND":
		return http.StatusNotFound
	case "ALREADY_EXISTS":
		return http.StatusConflict
	case "INVALID_INPUT":
		return http.StatusBadRequest
	case "INVALID_STATE":
		return http.StatusBadRequest
	case "UNAUTHORIZED":
		return http.StatusUnauthorized
	case "FORBIDDEN":
		return http.StatusForbidden
	case "CONCURRENCY":
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
