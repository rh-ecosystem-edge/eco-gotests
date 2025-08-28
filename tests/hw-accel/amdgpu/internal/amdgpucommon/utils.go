package amdgpucommon

import "github.com/golang/glog"

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string {
	return &s
}

// BoolPtr returns a pointer to the given bool.
func BoolPtr(b bool) *bool {
	return &b
}

// LogLevelPtr returns a pointer to the given glog.Level.
func LogLevelPtr(level glog.Level) *glog.Level {
	return &level
}
