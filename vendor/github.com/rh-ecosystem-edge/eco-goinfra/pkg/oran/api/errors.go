package api

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
)

// unimplementedError is an opaque error type that indicates that a method is not implemented for a specific client
// type. It is used when an interface must be implemented but not all interface methods can be implemented.
type unimplementedError struct {
	clientType ClientType
	method     string
}

func (err *unimplementedError) Error() string {
	return fmt.Sprintf("unimplemented: the %s client does not support calling the %s methods", err.clientType, err.method)
}

// Is checks to see if the provided error is an unimplementedError, defined by being able to assert the error to be
// *unimplementedError.
func (err *unimplementedError) Is(target error) bool {
	_, ok := target.(*unimplementedError)

	return ok
}

// Error is a general error type that gets returned when an API call fails.
type Error common.ProblemDetails

// AsAPIError checks to see if the error can be unwrapped to an *Error type. If it can, it returns the *Error.
// Otherwise, it returns nil.
func AsAPIError(err error) *Error {
	var apiError *Error
	if errors.As(err, &apiError) {
		return apiError
	}

	return nil
}

func (err *Error) Error() string {
	if err.Title != nil {
		return fmt.Sprintf("%d %s: %s", err.Status, *err.Title, err.Detail)
	}

	return fmt.Sprintf("%d %s", err.Status, err.Detail)
}

// Is checks to see if the provided error is an *Error type, defined by being able to assert the error to be *Error.
func (err *Error) Is(target error) bool {
	_, ok := target.(*Error)

	return ok
}

// apiErrorFromResponse uses reflection to convert the response to an *Error type. It does this by using the status code
// to see if a field named ApplicationProblemJSON<status code> exists. If it does, it tries to convert that field to a
// *ProblemDetails type. If it succeeds, it returns that as an *Error type.
//
// This method saves from having to use a switch statement on every API response to convert that response to an *Error
// type.
func apiErrorFromResponse[T interface{ StatusCode() int }](resp T) error {
	respValue := reflect.ValueOf(resp)
	if respValue.Kind() == reflect.Pointer && !respValue.IsNil() {
		respValue = respValue.Elem()
	}

	if respValue.Kind() != reflect.Struct {
		return fmt.Errorf("cannot create apiError from response: expected struct, got %s %T", respValue.Kind(), resp)
	}

	problemField := respValue.FieldByName(fmt.Sprintf("ApplicationProblemJSON%d", resp.StatusCode()))
	if !problemField.IsValid() {
		return fmt.Errorf(
			"cannot create apiError from response: field ApplicationProblemJSON%d does not exist", resp.StatusCode())
	}

	problemDetails, ok := problemField.Interface().(*common.ProblemDetails)
	if !ok {
		return fmt.Errorf(
			"cannot create apiError from response: field ApplicationProblemJSON%d is type %T, not *ProblemDetails",
			resp.StatusCode(), problemField.Interface())
	}

	return (*Error)(problemDetails)
}
