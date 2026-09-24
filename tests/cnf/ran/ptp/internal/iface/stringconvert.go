package iface

import "iter"

// NamesToStrings converts a slice of interface names to a slice of strings. It does this with a single allocation.
func NamesToStrings(names []Iface) []string {
	strings := make([]string, 0, len(names))

	for _, name := range names {
		strings = append(strings, string(name))
	}

	return strings
}

// NamesToStringSeq converts a sequence of interface names to a sequence of strings. It does this by returning a new
// sequence that yields from the provided sequence.
func NamesToStringSeq(names iter.Seq[Iface]) iter.Seq[string] {
	return func(yield func(string) bool) {
		for name := range names {
			if !yield(string(name)) {
				return
			}
		}
	}
}
