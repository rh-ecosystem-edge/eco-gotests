package api

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/fields"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
)

// ListOption configures optional query parameters for O2IMS list endpoints.
type ListOption interface {
	applyListQuery(*listQuery)
}

type listQuery struct {
	filter        *common.Filter
	fields        *common.Fields
	excludeFields *common.ExcludeFields
	allFields     *common.AllFields
}

type filterListOption struct {
	filter filter.Filter
}

func (option filterListOption) applyListQuery(query *listQuery) {
	query.filter = new(option.filter.Filter())
}

type fieldsListOption struct {
	selection *fields.Selection
}

func (option fieldsListOption) applyListQuery(query *listQuery) {
	query.fields, query.excludeFields, query.allFields = option.selection.Params()
}

// WithFilter adds a filter to a list request. If multiple WithFilter options are provided, the last one wins.
// Use filter.And() to combine multiple filter criteria.
//
//nolint:ireturn // list options are intentionally returned as the ListOption interface.
func WithFilter(f filter.Filter) ListOption {
	return filterListOption{filter: f}
}

// WithFields adds field selection to a list request. If multiple WithFields options are provided, the last one wins.
//
//nolint:ireturn // list options are intentionally returned as the ListOption interface.
func WithFields(selection *fields.Selection) ListOption {
	return fieldsListOption{selection: selection}
}

func applyListOptions(opts ...ListOption) listQuery {
	var query listQuery

	for _, option := range opts {
		option.applyListQuery(&query)
	}

	return query
}

func (q listQuery) hasOptions() bool {
	return q.filter != nil || q.fields != nil || q.excludeFields != nil || q.allFields != nil
}
