package fields

import (
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
)

// Selection configures fields, exclude_fields, and all_fields query parameters for O2IMS list endpoints.
type Selection struct {
	include []string
	exclude []string
	all     bool
}

// Include returns a selection that includes only the given field references in the response.
func Include(names ...string) *Selection {
	return &Selection{include: names}
}

// Exclude returns a selection that excludes the given field references from the response.
func Exclude(names ...string) *Selection {
	return &Selection{exclude: names}
}

// All returns a selection that requests all complex attributes in the response.
func All() *Selection {
	return &Selection{all: true}
}

// Path joins field name parts into a nested field reference.
func Path(parts ...string) string {
	return strings.Join(parts, "/")
}

// WithExclude returns a copy of the selection with additional fields to exclude.
func (s *Selection) WithExclude(names ...string) *Selection {
	if s == nil {
		return Exclude(names...)
	}

	selection := *s
	selection.exclude = append(append([]string(nil), s.exclude...), names...)

	return &selection
}

// Params converts the selection into the generated API parameter types.
func (s *Selection) Params() (fields *common.Fields, excludeFields *common.ExcludeFields, allFields *common.AllFields) {
	if s == nil {
		return nil, nil, nil
	}

	if len(s.include) > 0 {
		joined := strings.Join(s.include, ",")
		fields = &joined
	}

	if len(s.exclude) > 0 {
		joined := strings.Join(s.exclude, ",")
		excludeFields = &joined
	}

	if s.all {
		allFields = new(common.AllFields(""))
	}

	return fields, excludeFields, allFields
}
