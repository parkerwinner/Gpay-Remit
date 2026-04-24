package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode is a string representation of the error type
type ErrorCode string

const (
	CodeInternal      ErrorCode = "INTERNAL_ERROR"
	CodeValidation    ErrorCode = "VALIDATION_ERROR"
	CodeNotFound      ErrorCode = "NOT_FOUND"
	CodeUnauthorized  ErrorCode = "UNAUTHORIZED"
	CodeForbidden     ErrorCode = "FORBIDDEN"
	CodeConflict      ErrorCode = "CONFLICT"
)

// AppError represents a standardized application error
type AppError struct {
	Code       ErrorCode   `json:"code"`
	Message    string      `json:"message"`
	HTTPStatus int         `json:"-"`
	Details    interface{} `json:"details,omitempty"`
	Err        error       `json:"-"` // Underlying error for logging
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// NewAppError creates a new AppError
func NewAppError(status int, code ErrorCode, message string, err error, details interface{}) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Err:        err,
		Details:    details,
	}
}

// Helper functions for common errors

func NewInternalError(message string, err error) *AppError {
	if message == "" {
		message = "An unexpected error occurred"
	}
	return NewAppError(http.StatusInternalServerError, CodeInternal, message, err, nil)
}

func NewValidationError(message string, details interface{}) *AppError {
	return NewAppError(http.StatusBadRequest, CodeValidation, message, nil, details)
}

func NewNotFoundError(message string) *AppError {
	return NewAppError(http.StatusNotFound, CodeNotFound, message, nil, nil)
}

func NewUnauthorizedError(message string) *AppError {
	return NewAppError(http.StatusUnauthorized, CodeUnauthorized, message, nil, nil)
}

func NewForbiddenError(message string) *AppError {
	return NewAppError(http.StatusForbidden, CodeForbidden, message, nil, nil)
}

func NewConflictError(message string) *AppError {
	return NewAppError(http.StatusConflict, CodeConflict, message, nil, nil)
}
