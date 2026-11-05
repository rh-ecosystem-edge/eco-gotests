/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package typederrors

import (
	"errors"
	"fmt"
)

// InputError wraps a standard error and provides a custom error type for input-related errors
type InputError struct {
	err error
}

func (i *InputError) Error() string {
	return i.err.Error()
}

func NewInputError(format string, args ...interface{}) *InputError {
	return &InputError{
		err: fmt.Errorf(format, args...),
	}
}

func IsInputError(err error) bool {
	var inputErr *InputError

	return errors.As(err, &inputErr)
}
