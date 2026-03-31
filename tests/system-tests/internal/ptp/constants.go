package ptp

// Exported names for openshift-ptp / linuxptp daemon (reuse across PTP system tests).
const (
	Namespace                  = "openshift-ptp"
	DaemonContainerName        = "linuxptp-daemon-container"
	DaemonPodLabelKey          = "app"
	DaemonPodLabelValueLinuxpt = "linuxptp-daemon"
)

// Intel GNSS plugin keys in PtpConfig profiles and matching ubxtool -P protocol revisions.
const (
	PluginNameE825         = "e825"
	PluginNameE830         = "e830"
	PluginNameE810         = "e810"
	UbloxProtocolE825E830  = "29.25"
	UbloxProtocolE810      = "29.20"
	DefaultMaxAbsOffsetNS  = 100
	StateSubscribedLogMark = " s2"
	LogKeywordHoldover     = "holdover"
	LogKeywordFreerun      = "freerun"
)
