package o2imstest

import "fmt"

// ExtensionString returns extensions[key] when it is a string, or "" otherwise.
func ExtensionString(extensions map[string]any, key string) string {
	value, ok := extensions[key].(string)
	if !ok {
		return ""
	}

	return value
}

// AsStringKeyedMap converts common JSON-decoded map shapes to map[string]string.
func AsStringKeyedMap(value any) (map[string]string, bool) {
	switch typed := value.(type) {
	case map[string]string:
		return typed, true
	case map[string]any:
		result := make(map[string]string, len(typed))
		for key, nested := range typed {
			result[key] = fmt.Sprint(nested)
		}

		return result, true
	default:
		return nil, false
	}
}
