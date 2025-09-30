package metrics

// PtpMetricKey is an enum representing all possible keys for labels on PTP metrics.
type PtpMetricKey string

//nolint:revive // The key names are self explanatory and do not need individual comments.
const (
	KeyProcess   PtpMetricKey = "process"
	KeyInterface PtpMetricKey = "iface"
	KeyNode      PtpMetricKey = "node"
	KeyConfig    PtpMetricKey = "config"
	KeyProfile   PtpMetricKey = "profile"
	KeyThreshold PtpMetricKey = "threshold"
	KeyFrom      PtpMetricKey = "from"
)

// PtpMetric is an enum representing all PTP metrics supported as typed queries.
type PtpMetric string

//nolint:revive // The metric names are self explanatory and do not need individual comments.
const (
	MetricClockState      PtpMetric = "openshift_ptp_clock_state"
	MetricProcessStatus   PtpMetric = "openshift_ptp_process_status"
	MetricInterfaceRole   PtpMetric = "openshift_ptp_interface_role"
	MetricThreshold       PtpMetric = "openshift_ptp_threshold"
	MetricNMEAStatus      PtpMetric = "openshift_ptp_nmea_status"
	MetricHAProfileStatus PtpMetric = "openshift_ptp_ha_profile_status"
	MetricPPSStatus       PtpMetric = "openshift_ptp_pps_status"
	MetricClockClass      PtpMetric = "openshift_ptp_clock_class"
)

// PtpClockState is an enum representing all possible states of the PTP clock.
type PtpClockState int

const (
	// ClockStateFreerun is the state of the PTP clock when it is not synchronized to a time transmitter.
	ClockStateFreerun PtpClockState = iota
	// ClockStateLocked is the state of the PTP clock when it is synchronized to a time transmitter.
	ClockStateLocked
	// ClockStateHoldover is the state of the PTP clock when it is in holdover mode, meaning it is using its
	// internal clock to maintain time when it is not receiving a signal from a time transmitter.
	ClockStateHoldover
)

// PtpProcessStatus is an enum representing all possible states of the PTP process.
type PtpProcessStatus int

//nolint:revive // The process status names are self explanatory and do not need individual comments.
const (
	ProcessStatusDown PtpProcessStatus = iota
	ProcessStatusUp
)

// PtpInterfaceRole is an enum representing all possible roles of the PTP interface.
type PtpInterfaceRole int

const (
	// InterfaceRolePassive is the role of the PTP interface when it is neither a leader nor a follower.
	InterfaceRolePassive PtpInterfaceRole = iota
	// InterfaceRoleFollower is the role of the PTP interface when it is receiving time from a leader clock. Also
	// called slave.
	InterfaceRoleFollower
	// InterfaceRoleLeader is the role of the PTP interface when it is transmitting time to other follower clocks.
	// Also called master.
	InterfaceRoleLeader
	// InterfaceRoleFaulty is the role of the PTP interface when it is not functioning correctly.
	InterfaceRoleFaulty
	// InterfaceRoleUnknown means the interface role cannot be determined.
	InterfaceRoleUnknown
	// InterfaceRoleListening is the role of the PTP interface when it is configured as a follower but is not
	// syncing with the PHC. This is used in the dual follower (two port OC) configuration for the second follower.
	InterfaceRoleListening
)

// PtpThresholdType is an enum representing all possible types of PTP thresholds. It corresponds to the keys of
// ptpv1.PtpClockThresholds.
type PtpThresholdType string

const (
	// ThresholdHoldoverTimeout is the time a clock stays in holdover before transitioning to freerun after losing
	// connection to a time source.
	ThresholdHoldoverTimeout PtpThresholdType = "HoldOverTimeout"
	// ThresholdMaxOffset is the maximum offset allowed before the clock enters holdover. This is usually a positive
	// value.
	ThresholdMaxOffset PtpThresholdType = "MaxOffsetThreshold"
	// ThresholdMinOffset is the minimum offset allowed before the clock enters holdover. This is usually a negative
	// value and defaults to the negative of the max offset.
	ThresholdMinOffset PtpThresholdType = "MinOffsetThreshold"
)

// PtpNMEAStatus is an enum representing all possible states of the PTP NMEA status. It is similar to PtpProcessStatus
// but typed specifically for the NMEA status metric.
type PtpNMEAStatus int

//nolint:revive // The NMEA status names are self explanatory and do not need individual comments.
const (
	NMEAStatusUnavailable PtpNMEAStatus = iota
	NMEAStatusAvailable
)

// PtpHAProfileStatus is an enum representing all possible states of the PTP HA profile status. It is similar to
// PtpProcessStatus but typed specifically for the HA profile status metric.
type PtpHAProfileStatus int

//nolint:revive // The HA profile status names are self explanatory and do not need individual comments.
const (
	HAProfileStatusInactive PtpHAProfileStatus = iota
	HAProfileStatusActive
)

// PtpPPSStatus is an enum representing all possible states of the PTP PPS status. It is similar to PtpProcessStatus but
// typed specifically for the PPS status metric.
type PtpPPSStatus int

//nolint:revive // The PPS status names are self explanatory and do not need individual comments.
const (
	PPSStatusUnavailable PtpPPSStatus = iota
	PPSStatusAvailable
)

// PtpProcess is an enum representing all possible values for PTP processes. This is used as the type for the from label
// and the process label.
type PtpProcess string

//nolint:revive // The process names are self explanatory and do not need individual comments.
const (
	ProcessPTP4L   PtpProcess = "ptp4l"
	ProcessPHC2SYS PtpProcess = "phc2sys"
	ProcessTS2PHC  PtpProcess = "ts2phc"
	ProcessGPSD    PtpProcess = "gpsd"
	ProcessGPSPIPE PtpProcess = "gpspipe"
	ProcessDPLL    PtpProcess = "dpll"
	ProcessGNSS    PtpProcess = "gnss"
	ProcessGM      PtpProcess = "GM"
)
