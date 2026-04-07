package middleware

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/duragraph/duragraph/internal/infrastructure/http/dto"
	"github.com/duragraph/duragraph/internal/pkg/errors"
	"github.com/labstack/echo/v4"
)

func ErrorHandler() echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		reqID, _ := c.Get("request_id").(string)

		var domainErr *errors.DomainError
		if errors.As(err, &domainErr) {
			statusCode := mapDomainErrorToHTTPStatus(domainErr)
			resp := dto.ErrorResponse{
				Error:   domainErr.Code,
				Message: domainErr.Message,
				Code:    domainErr.Code,
			}
			if reqID != "" {
				resp.RequestID = reqID
			}
			logError(c, statusCode, domainErr.Code, domainErr.Message, reqID)
			c.JSON(statusCode, resp)
			return
		}

		if he, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", he.Message)
			resp := dto.ErrorResponse{
				Error:   http.StatusText(he.Code),
				Message: msg,
			}
			if reqID != "" {
				resp.RequestID = reqID
			}
			logError(c, he.Code, http.StatusText(he.Code), msg, reqID)
			c.JSON(he.Code, resp)
			return
		}

		safeMsg := sanitizeErrorMessage(err.Error())
		resp := dto.ErrorResponse{
			Error:   "internal_error",
			Message: safeMsg,
		}
		if reqID != "" {
			resp.RequestID = reqID
		}
		log.Printf("[ERROR] request_id=%s method=%s path=%s status=500 error=%q",
			reqID, c.Request().Method, c.Request().URL.Path, err.Error())
		c.JSON(http.StatusInternalServerError, resp)
	}
}

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
	case "TIMEOUT":
		return http.StatusGatewayTimeout
	case "RATE_LIMITED":
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

func sanitizeErrorMessage(msg string) string {
	lower := strings.ToLower(msg)
	for _, keyword := range []string{"password", "secret", "token", "key", "credential", "dsn", "connection string"} {
		if strings.Contains(lower, keyword) {
			return "An internal error occurred"
		}
	}
	if len(msg) > 500 {
		return msg[:500]
	}
	return msg
}

func logError(c echo.Context, status int, code, message, reqID string) {
	if status >= 500 {
		log.Printf("[ERROR] request_id=%s method=%s path=%s status=%d code=%s message=%q",
			reqID, c.Request().Method, c.Request().URL.Path, status, code, message)
	}
}
