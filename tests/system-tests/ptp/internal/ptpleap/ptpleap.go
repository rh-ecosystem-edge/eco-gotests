package ptpleap

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// announcementPattern is a regular expression that matches the last leap event announcement.
// An example of an announcement is: "\n3692217600     37    # 1 Jan 2017".
var announcementPattern = regexp.MustCompile(`\n(\d+\s+\d+\s+#\s\d+\s[a-zA-Z]+\s\d{4})\n\n`)

// leapLinePattern is a regular expression that matches the last line of the leap event announcement.
// An example of a leap line is: "3692217600     37    #".
var leapLinePattern = regexp.MustCompile(`^\s*\d+\s+\d+\s+#`)

// GetLastAnnouncement returns the last leap event announcement from a leap-configmap Data.
func GetLastAnnouncement(leapConfigMapData string) (string, error) {
	if len(leapConfigMapData) == 0 {
		return leapConfigMapData, nil
	}

	announcementSlice := announcementPattern.FindStringSubmatch(leapConfigMapData)

	if len(announcementSlice) < 2 {
		return "", fmt.Errorf("error finding the last announcement")
	}

	return announcementSlice[1], nil
}

// RemoveLastLeapAnnouncement removes the last "leap announcement" line,
// i.e., the last line that looks like: "<seconds> <offset> # <date>".
func RemoveLastLeapAnnouncement(s string) string {
	lines := strings.Split(s, "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		if leapLinePattern.MatchString(lines[i]) {
			lines = append(lines[:i], lines[i+1:]...)

			break
		}
	}

	return strings.Join(lines, "\n")
}

// WaitForConfigmapToBeUpdated waits until the configmap is updated with the last leap announcement line
// that matches today's date in UTC, formatted "d Mon yyyy".
func WaitForConfigmapToBeUpdated(interval time.Duration,
	timeout time.Duration) error {
	err := wait.PollUntilContextTimeout(
		context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
			today := time.Now().UTC().Format("2 Jan 2006")

			leapConfigMap, err := configmap.Pull(
				RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
			if err != nil {
				klog.Errorf("failed to pull leap configmap %s/%s: %v",
					ranparam.PtpOperatorNamespace, tsparams.LeapConfigmapName, err)

				return false, nil
			}

			for _, leapConfigmapData := range leapConfigMap.Object.Data {
				if strings.Contains(leapConfigmapData, today) {
					return true, nil
				}
			}

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("failed waiting for configmap %s/%s to be updated with today's leap event: %w",
			ranparam.PtpOperatorNamespace, tsparams.LeapConfigmapName, err)
	}

	return nil
}
