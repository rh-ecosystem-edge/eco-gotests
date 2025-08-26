package search

import (
	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
)

// StringInSlice check if match string is exist in string slice/array.
func StringInSlice(match string, listOfString []string) bool {
	glog.V(nfdparams.LogLevel).Infof("verify that %s exist %v", match, listOfString)

	for _, str := range listOfString {
		if str == match {
			return true
		}
	}

	return false
}
