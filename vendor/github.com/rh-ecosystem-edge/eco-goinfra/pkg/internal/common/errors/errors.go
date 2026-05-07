// Package errors provides the error types for the common package. It is currently meant just for use in the common
// package and therefore focuses on the errors encountered in the common package.
package errors

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/key"
)

type apiClientNilError struct {
	resourceKey key.ResourceKey
}

var _ error = (*apiClientNilError)(nil)

// NewAPIClientNil creates a new error that indicates that the apiClient for a builder is nil.
func NewAPIClientNil(resourceKey key.ResourceKey) *apiClientNilError {
	return &apiClientNilError{resourceKey: resourceKey}
}

func (e *apiClientNilError) Error() string {
	return fmt.Sprintf("apiClient for %s is nil", e.resourceKey.String())
}

// IsAPIClientNil returns true if an error, or any error in the error's tree, is due to the apiClient being nil.
func IsAPIClientNil(err error) bool {
	var apiClientNilError *apiClientNilError

	return errors.As(err, &apiClientNilError)
}

type schemeAttacherFailedError struct {
	resourceKey key.ResourceKey
	err         error
}

var _ error = (*schemeAttacherFailedError)(nil)

// NewSchemeAttacherFailed creates a new error that indicates that the scheme attacher failed to attach. It wraps the
// error returned by the scheme attacher function.
func NewSchemeAttacherFailed(resourceKey key.ResourceKey, err error) *schemeAttacherFailedError {
	return &schemeAttacherFailedError{resourceKey: resourceKey, err: err}
}

func (e *schemeAttacherFailedError) Error() string {
	return fmt.Sprintf("failed to attach scheme for %s: %v", e.resourceKey.String(), e.err)
}

func (e *schemeAttacherFailedError) Unwrap() error {
	return e.err
}

// IsSchemeAttacherFailed returns true if an error, or any error in the error's tree, is due to the scheme attacher
// failing to attach.
func IsSchemeAttacherFailed(err error) bool {
	var schemeAttacherFailed *schemeAttacherFailedError

	return errors.As(err, &schemeAttacherFailed)
}

type builderFieldEmptyError struct {
	resourceKey key.ResourceKey
	field       BuilderField
}

var _ error = (*builderFieldEmptyError)(nil)

// BuilderField is a type that represents a field for a builder.
type BuilderField string

const (
	// BuilderFieldName is the name of the field for a builder's name. This corresponds to the Name field of the
	// ObjectMeta.
	BuilderFieldName BuilderField = "name"
	// BuilderFieldNamespace is the namespace of the field for a builder's namespace. This corresponds to the
	// Namespace field of the ObjectMeta.
	BuilderFieldNamespace BuilderField = "namespace"
)

// NewBuilderFieldEmpty creates a new error that indicates that a field for a builder is empty.
func NewBuilderFieldEmpty(resourceKey key.ResourceKey, field BuilderField) *builderFieldEmptyError {
	return &builderFieldEmptyError{resourceKey: resourceKey, field: field}
}

func (e *builderFieldEmptyError) Error() string {
	return fmt.Sprintf("%s of the builder for %s is empty", e.field, e.resourceKey.String())
}

// IsBuilderNameEmpty returns true if an error, or any error in the error's tree, is due to the builder's name being
// empty.
func IsBuilderNameEmpty(err error) bool {
	var builderFieldEmpty *builderFieldEmptyError

	return errors.As(err, &builderFieldEmpty) && builderFieldEmpty.field == BuilderFieldName
}

// IsBuilderNamespaceEmpty returns true if an error, or any error in the error's tree, is due to the builder's namespace
// being empty.
func IsBuilderNamespaceEmpty(err error) bool {
	var builderFieldEmpty *builderFieldEmptyError

	return errors.As(err, &builderFieldEmpty) && builderFieldEmpty.field == BuilderFieldNamespace
}

type apiCallFailedError struct {
	verb        string
	resourceKey key.ResourceKey
	err         error
}

var _ error = (*apiCallFailedError)(nil)

// NewAPICallFailed creates a new error that indicates that an API call failed. The verb is not validated, but intended
// to correspond to the basic methods in the Kubernetes interface, such as Get, List, Create, etc.
func NewAPICallFailed(verb string, resourceKey key.ResourceKey, err error) *apiCallFailedError {
	return &apiCallFailedError{verb: verb, resourceKey: resourceKey, err: err}
}

func (e *apiCallFailedError) Error() string {
	return fmt.Sprintf("failed to %s %s: %v", e.verb, e.resourceKey.String(), e.err)
}

func (e *apiCallFailedError) Unwrap() error {
	return e.err
}

// IsAPICallFailed returns true if an error, or any error in the error's tree, is due to an API call failing.
func IsAPICallFailed(err error) bool {
	var apiCallFailed *apiCallFailedError

	return errors.As(err, &apiCallFailed)
}

// IsAPICallFailedWithVerb returns true if an error, or any error in the error's tree, is due to an API call failing
// with the given verb.
func IsAPICallFailedWithVerb(err error, verb string) bool {
	var apiCallFailed *apiCallFailedError

	return errors.As(err, &apiCallFailed) && apiCallFailed.verb == verb
}

type builderNilError struct{}

var _ error = (*builderNilError)(nil)

// NewBuilderNil creates a new error that indicates that the builder is nil.
func NewBuilderNil() *builderNilError {
	return &builderNilError{}
}

func (e *builderNilError) Error() string {
	return "builder is nil"
}

// IsBuilderNil returns true if an error, or any error in the error's tree, is due to the builder being nil.
func IsBuilderNil(err error) bool {
	var builderNil *builderNilError

	return errors.As(err, &builderNil)
}

type builderDefinitionNilError struct {
	kind string
}

var _ error = (*builderDefinitionNilError)(nil)

// NewBuilderDefinitionNil creates a new error that indicates that the builder's definition is nil.
func NewBuilderDefinitionNil(kind string) *builderDefinitionNilError {
	return &builderDefinitionNilError{kind: kind}
}

func (e *builderDefinitionNilError) Error() string {
	return fmt.Sprintf("%s builder definition is nil", e.kind)
}

// IsBuilderDefinitionNil returns true if an error, or any error in the error's tree, is due to the builder's definition
// being nil.
func IsBuilderDefinitionNil(err error) bool {
	var builderDefinitionNil *builderDefinitionNilError

	return errors.As(err, &builderDefinitionNil)
}

type itemTypeMismatchError struct {
	kind     string
	itemType reflect.Type
}

var _ error = (*itemTypeMismatchError)(nil)

// NewItemTypeMismatch creates a new error that indicates that an item type mismatch occurred.
func NewItemTypeMismatch(kind string, itemType reflect.Type) *itemTypeMismatchError {
	return &itemTypeMismatchError{kind: kind, itemType: itemType}
}

func (e *itemTypeMismatchError) Error() string {
	return fmt.Sprintf("item has kind %s but type %s", e.kind, e.itemType.String())
}

// IsItemTypeMismatch returns true if an error, or any error in the error's tree, is due to an item type mismatch.
func IsItemTypeMismatch(err error) bool {
	var itemTypeMismatch *itemTypeMismatchError

	return errors.As(err, &itemTypeMismatch)
}
