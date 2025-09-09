package filter

import "strings"

// FilterOperator is an enum that identifies all the possible operators for a filter. These values should likely not be
// used directly and instead the functions returning filters will include these constants.
type FilterOperator string

const (
	// FilterOperatorCont matches if the field contains the value.
	FilterOperatorCont FilterOperator = "cont"
	// FilterOperatorEq matches if the field is equal to the value.
	FilterOperatorEq FilterOperator = "eq"
	// FilterOperatorGt matches if the field is greater than the value.
	FilterOperatorGt FilterOperator = "gt"
	// FilterOperatorGte matches if the field is greater than or equal to the value.
	FilterOperatorGte FilterOperator = "gte"
	// FilterOperatorIn matches if the field is one of the values.
	FilterOperatorIn FilterOperator = "in"
	// FilterOperatorLt matches if the field is less than the value.
	FilterOperatorLt FilterOperator = "lt"
	// FilterOperatorLte matches if the field is less than or equal to the value.
	FilterOperatorLte FilterOperator = "lte"
	// FilterOperatorNcont matches if the field does not contain the value.
	FilterOperatorNcont FilterOperator = "ncont"
	// FilterOperatorNeq matches if the field is not equal to the value.
	FilterOperatorNeq FilterOperator = "neq"
	// FilterOperatorNin matches if the field is not one of the values.
	FilterOperatorNin FilterOperator = "nin"
)

// Filter represents any type that can be turned into a filter string for the O2IMS API.
type Filter interface {
	Filter() string
}

// Contains returns a filter that matches if the field contains the value.
func Contains(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorCont,
		Field:    field,
		Values:   []string{value},
	}
}

// Equals returns a filter that matches if the field is equal to the value.
func Equals(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorEq,
		Field:    field,
		Values:   []string{value},
	}
}

// GreaterThan returns a filter that matches if the field is greater than the value.
func GreaterThan(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorGt,
		Field:    field,
		Values:   []string{value},
	}
}

// GreaterThanOrEqual returns a filter that matches if the field is greater than or equal to the value.
func GreaterThanOrEqual(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorGte,
		Field:    field,
		Values:   []string{value},
	}
}

// In returns a filter that matches if the field is one of the values.
func In(field string, values ...string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorIn,
		Field:    field,
		Values:   values,
	}
}

// LessThan returns a filter that matches if the field is less than the value.
func LessThan(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorLt,
		Field:    field,
		Values:   []string{value},
	}
}

// LessThanOrEqual returns a filter that matches if the field is less than or equal to the value.
func LessThanOrEqual(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorLte,
		Field:    field,
		Values:   []string{value},
	}
}

// DoesNotContain returns a filter that matches if the field does not contain the value.
func DoesNotContain(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorNcont,
		Field:    field,
		Values:   []string{value},
	}
}

// DoesNotEqual returns a filter that matches if the field is not equal to the value.
func DoesNotEqual(field string, value string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorNeq,
		Field:    field,
		Values:   []string{value},
	}
}

// NotIn returns a filter that matches if the field is not one of the values.
func NotIn(field string, values ...string) *basicFilter {
	return &basicFilter{
		Operator: FilterOperatorNin,
		Field:    field,
		Values:   values,
	}
}

// And returns a filter that matches if all of the provided filters match.
func And(filters ...Filter) *andFilter {
	return (*andFilter)(&filters)
}

// basicFilter is a filter that contains only a single operator with its associated field and value(s).
type basicFilter struct {
	Operator FilterOperator
	Field    string
	Values   []string
}

// Assert at compile time that basicFilter implements the Filter interface.
var _ Filter = (*basicFilter)(nil)

// Filter returns the filter string.
func (f *basicFilter) Filter() string {
	var builder strings.Builder

	builder.WriteByte('(')
	builder.WriteString(string(f.Operator))
	builder.WriteByte(',')
	builder.WriteString(f.Field)
	builder.WriteByte(',')

	for i, value := range f.Values {
		// Add a comma separator between values, but no trailing comma.
		if i > 0 {
			builder.WriteByte(',')
		}

		// "When values contain commas, slashes or spaces they need to be surrounded by single quotes."
		if strings.ContainsAny(value, "(), ") {
			builder.WriteByte('\'')
			builder.WriteString(value)
			builder.WriteByte('\'')
		} else {
			builder.WriteString(value)
		}
	}

	builder.WriteByte(')')

	return builder.String()
}

// andFilter is a filter that contains multiple filters, all of which must match. It can compose basicFilters and other
// andFilters.
type andFilter []Filter

// Assert at compile time that andFilter implements the Filter interface.
var _ Filter = (*andFilter)(nil)

// Filter returns the filter string.
func (f *andFilter) Filter() string {
	var builder strings.Builder

	for i, filter := range *f {
		// Add a semicolon separator between filters, but no trailing semicolon.
		if i > 0 {
			builder.WriteByte(';')
		}

		builder.WriteString(filter.Filter())
	}

	return builder.String()
}
