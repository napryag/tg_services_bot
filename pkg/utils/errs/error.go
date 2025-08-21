package errs

import (
	"errors"
	"fmt"
	"strings"
)

// CustomError represents a custom error with additional arguments and wrapping capability.
type CustomError struct {
	message string
	args    map[string]interface{}
	wrapped error
}

// New creates a new CustomError instance.
func New(message string) *CustomError {
	return &CustomError{
		message: message,
		args:    make(map[string]interface{}),
	}
}

// Error implements the error interface.
func (e *CustomError) Error() string {
	return e.fullErrorString()
}

// Arg adds an argument to the error.
func (e *CustomError) Arg(key string, value interface{}) *CustomError {
	e.args[key] = value
	return e
}

// Wrap wraps another error (can be of the same type or a standard error).
func (e *CustomError) Wrap(err error) *CustomError {
	if err != nil {
		e.wrapped = err
	}
	return e
}

// Unwrap returns the wrapped error if any.
func (e *CustomError) Unwrap() error {
	return e.wrapped
}

// fullErrorString builds the error string in the desired format:
// "{msg: <message>, args: <args>, wrappedError: {<wrapped error>}}".
func (e *CustomError) fullErrorString() string {
	var builder strings.Builder

	// Start with the opening brace
	builder.WriteString("{msg: ")

	// Add the main message
	builder.WriteString(e.message)

	// Add arguments if they exist
	if len(e.args) > 0 {
		builder.WriteString(fmt.Sprintf(", args: %v", e.args))
	}

	// Add wrapped error if it exists
	if e.wrapped != nil {
		wrappedErr := &CustomError{}
		if errors.As(e.wrapped, &wrappedErr) {
			// If the wrapped error is also a CustomError, use its fullErrorString
			builder.WriteString(fmt.Sprintf(", wrappedError: %s", wrappedErr.fullErrorString()))
		} else {
			// Otherwise, wrap the error message in curly braces
			builder.WriteString(fmt.Sprintf(", wrappedError: {%v}", e.wrapped.Error()))
		}
	}

	// Close with the closing brace
	builder.WriteString("}")

	return builder.String()
}
