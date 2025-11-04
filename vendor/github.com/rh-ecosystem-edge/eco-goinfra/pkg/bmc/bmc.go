package bmc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/golang/glog"
	"github.com/stmcginnis/gofish"
	"github.com/stmcginnis/gofish/redfish"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// defaultSSHPort is the default port that will be used for SSH connections.
	defaultSSHPort = 22
	defaultTimeOut = 5 * time.Second

	manufacturerDell = "Dell Inc."
	manufacturerHPE  = "HPE"
)

var (
	// DefaultTimeOuts holds the default redfish and ssh timeouts.
	DefaultTimeOuts = TimeOuts{
		Redfish: defaultTimeOut,
		SSH:     defaultTimeOut,
	}

	// CLI command to get the serial console (virtual serial port).
	cliCmdSerialConsole = map[string]string{
		manufacturerHPE:  "VSP",
		manufacturerDell: "console com2",
	}
)

// User holds the Name and Password for a user (ssh/redfish).
type User struct {
	// Name holds the user's name
	Name string
	// Password holds the user's password
	Password string
}

// TimeOuts holds the configured timeouts for Redfish and SSH acccess.
type TimeOuts struct {
	// Redfish timeout for the redfish api access.
	Redfish time.Duration
	// SSH timeout for the ssh access.
	SSH time.Duration
}

// BMC is the holder struct for BMC access through redfish & ssh.
type BMC struct {
	host        string
	redfishUser *User
	sshUser     *User
	sshPort     uint16
	timeOuts    TimeOuts

	systemIndex       int
	powerControlIndex int

	sshClientForSerialConsole *ssh.Client

	errorMsg string
}

// New returns a BMC struct with the specified host. The host should be nonempty. WithRedfishUser and WithSSHUser must
// be called before connecting to Redfish or over SSH, respectively. The SSH port and timeouts are set to DefaultSSHPort
// and DefaultTimeOuts, with indices defaulting to 0.
func New(host string) *BMC {
	glog.V(100).Infof(
		"Creating new BMC structure with the following params: host: %s", host)

	bmc := &BMC{
		host:              host,
		sshPort:           defaultSSHPort,
		timeOuts:          DefaultTimeOuts,
		systemIndex:       0,
		powerControlIndex: 0,
	}

	if host == "" {
		glog.V(100).Info("The host of the BMC is empty")

		bmc.errorMsg = "bmc 'host' cannot be empty"
	}

	return bmc
}

// WithRedfishUser provides the credentials to access the Redfish API. Neither the username nor password should be
// empty.
func (bmc *BMC) WithRedfishUser(username, password string) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	glog.V(100).Infof("Setting BMC Redfish username to %s", username)

	if username == "" {
		glog.V(100).Info("The Redfish username is empty")

		bmc.errorMsg = "redfish 'username' cannot be empty"

		return bmc
	}

	if password == "" {
		glog.V(100).Info("The Redfish password is empty")

		bmc.errorMsg = "redfish 'password' cannot be empty"

		return bmc
	}

	bmc.redfishUser = &User{
		Name:     username,
		Password: password,
	}

	return bmc
}

// WithRedfishTimeout provides the timeout to use when connecting to the Redfish API. It should not be zero or negative.
func (bmc *BMC) WithRedfishTimeout(timeout time.Duration) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	if timeout <= 0 {
		glog.V(100).Infof("The Redfish timeout %s is less than or equal to zero", timeout)

		bmc.errorMsg = "redfish 'timeout' cannot be less than or equal to zero"

		return bmc
	}

	bmc.timeOuts.Redfish = timeout

	return bmc
}

// WithRedfishSystemIndex provies the index of the system to use in the Redfish API. Note that the order of the systems
// is nondeterministic.
func (bmc *BMC) WithRedfishSystemIndex(index int) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	if index < 0 {
		glog.V(100).Infof("The Redfish System index is negative: %d", index)

		bmc.errorMsg = "redfish 'systemIndex' cannot be negative"

		return bmc
	}

	bmc.systemIndex = index

	return bmc
}

// WithRedfishPowerControlIndex provides the index of the PowerControl object to use from the Power link on the Chassis
// service in the Redfish API. The order of the PowerControl objects is deterministic.
func (bmc *BMC) WithRedfishPowerControlIndex(index int) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	if index < 0 {
		glog.V(100).Infof("The Redfish PowerControl index is negative: %d", index)

		bmc.errorMsg = "redfish 'powerControlIndex' cannot be negative"

		return bmc
	}

	bmc.powerControlIndex = index

	return bmc
}

// WithSSHUser provides the credentials to use when connecting to the BMC over SSH. Neither the username nor the
// password should be empty.
func (bmc *BMC) WithSSHUser(username, password string) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	glog.V(100).Infof("Setting BMC SSH username to %s", username)

	if username == "" {
		glog.V(100).Info("The SSH username is empty")

		bmc.errorMsg = "ssh 'username' cannot be empty"

		return bmc
	}

	if password == "" {
		glog.V(100).Info("The SSH password is empty")

		bmc.errorMsg = "ssh 'password' cannot be empty"

		return bmc
	}

	bmc.sshUser = &User{
		Name:     username,
		Password: password,
	}

	return bmc
}

// WithSSHPort provides the port to use when connecting to the BMC over SSH. It should not be zero.
func (bmc *BMC) WithSSHPort(port uint16) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	glog.V(100).Infof("Setting SSH port to %d", port)

	if port == 0 {
		glog.V(100).Infof("The SSH port is zero")

		bmc.errorMsg = "ssh 'port' cannot be zero"

		return bmc
	}

	bmc.sshPort = port

	return bmc
}

// WithSSHTimeout provides the timeout to use when connecting to the BMC over SSH. It should not be zero or negative.
func (bmc *BMC) WithSSHTimeout(timeout time.Duration) *BMC {
	if valid, _ := bmc.validate(); !valid {
		return bmc
	}

	if timeout <= 0 {
		glog.V(100).Infof("The SSH timeout %s is less than or equal to zero", timeout)

		bmc.errorMsg = "ssh 'timeout' cannot be less than or equal to zero"

		return bmc
	}

	bmc.timeOuts.SSH = timeout

	return bmc
}

// SystemManufacturer gets system's manufacturer from the BMC's RedFish API endpoint.
func (bmc *BMC) SystemManufacturer() (string, error) {
	if valid, err := bmc.validateRedfish(); !valid {
		return "", err
	}

	glog.V(100).Infof("Getting SystemManufacturer param from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return "", fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return "", fmt.Errorf("failed to get redfish system: %w", err)
	}

	return system.Manufacturer, nil
}

// IsSecureBootEnabled returns whether the SecureBoot feature is enabled using the BMC's RedFish API endpoint.
func (bmc *BMC) IsSecureBootEnabled() (bool, error) {
	if valid, err := bmc.validateRedfish(); !valid {
		return false, err
	}

	glog.V(100).Infof("Getting secure boot status from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return false, fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	sboot, err := redfishGetSystemSecureBoot(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system's secure boot: %v", err)

		return false, fmt.Errorf("failed to get secure boot: %w", err)
	}

	return sboot.SecureBootEnable, nil
}

// SecureBootEnable enables the SecureBoot feature using the BMC's RedFish API endpoint.
func (bmc *BMC) SecureBootEnable() error {
	if valid, err := bmc.validateRedfish(); !valid {
		return err
	}

	glog.V(100).Infof("Enabling secure boot from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	sboot, err := redfishGetSystemSecureBoot(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system's secure boot: %v", err)

		return fmt.Errorf("failed to get secure boot: %w", err)
	}

	if sboot.SecureBootEnable {
		glog.V(100).Infof("Failed to enable secure boot: it is already enabled")

		return fmt.Errorf("secure boot is already enabled")
	}

	sboot.SecureBootEnable = true

	err = sboot.Update()
	if err != nil {
		glog.V(100).Infof("Failed to enable secure boot: %v", err)

		return fmt.Errorf("failed to enable secure boot: %w", err)
	}

	return nil
}

// SecureBootDisable disables the SecureBoot feature using the BMC's RedFish API endpoint.
func (bmc *BMC) SecureBootDisable() error {
	if valid, err := bmc.validateRedfish(); !valid {
		return err
	}

	glog.V(100).Infof("Disabling secure boot from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	sboot, err := redfishGetSystemSecureBoot(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system's secure boot: %v", err)

		return fmt.Errorf("failed to get secure boot: %w", err)
	}

	if !sboot.SecureBootEnable {
		glog.V(100).Infof("Failed to disable secure boot: it is already disabled")

		return fmt.Errorf("secure boot is already disabled")
	}

	sboot.SecureBootEnable = false

	err = sboot.Update()
	if err != nil {
		glog.V(100).Infof("Failed to disable secure boot: %v", err)

		return fmt.Errorf("failed to disable secure boot: %w", err)
	}

	return nil
}

// SystemResetAction performs the specified reset action against the system.
func (bmc *BMC) SystemResetAction(action redfish.ResetType) error {
	if valid, err := bmc.validateRedfish(); !valid {
		return err
	}

	glog.V(100).Infof("Performing reset action %v from the bmc's redfish endpoint", action)

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return fmt.Errorf("failed to get redfish system: %w", err)
	}

	return system.Reset(action)
}

// SystemForceReset performs a (non-graceful) forced system reset using Redfish API.
func (bmc *BMC) SystemForceReset() error {
	return bmc.SystemResetAction(redfish.ForceRestartResetType)
}

// SystemGracefulShutdown performs a graceful shutdown using the Redfish API.
func (bmc *BMC) SystemGracefulShutdown() error {
	return bmc.SystemResetAction(redfish.GracefulShutdownResetType)
}

// SystemPowerOn powers on the system using the Redfish API.
func (bmc *BMC) SystemPowerOn() error {
	return bmc.SystemResetAction(redfish.OnResetType)
}

// SystemPowerOff performs a non-graceful power off of the system using the Redfish API.
func (bmc *BMC) SystemPowerOff() error {
	return bmc.SystemResetAction(redfish.ForceOffResetType)
}

// SystemPowerCycle performs a power cycle in the system using the Redfish API. If PowerCycle reset type
// is not supported, alternate PowerOff + On reset actions will be performed as fallback mechanism.
// Use bmc.SystemResetAction(redfish.PowerCycleResetType) if this fallback mechanism is not needed/wanted.
func (bmc *BMC) SystemPowerCycle() error {
	if valid, err := bmc.validateRedfish(); !valid {
		return err
	}

	glog.V(100).Infof("Checking whether PowerCycle reset type can be performed from the bmc's redfish endpoint")

	suppportedResetTypes, err := bmc.getSupportedResetTypes()
	if err != nil {
		glog.V(100).Infof("Failed to get system's supported reset types: %v", err)

		return fmt.Errorf("failed to get system's supported reset types: %w", err)
	}

	// If supported, perform power cycle reset.
	if isResetTypeSupported(redfish.PowerCycleResetType, suppportedResetTypes) {
		return bmc.SystemResetAction(redfish.PowerCycleResetType)
	}

	glog.V(100).Infof("PowerCycle reset type not supported. Trying with PowerOff and On reset actions")

	// Workaround for PowerCycle type not supported: ForceOff + On.
	if !isResetTypeSupported(redfish.ForceOffResetType, suppportedResetTypes) ||
		!isResetTypeSupported(redfish.OnResetType, suppportedResetTypes) {
		glog.V(100).Infof("Unable to perform power cycle (supported reset types: %v)", suppportedResetTypes)

		return fmt.Errorf("unable to perform power cycle (supported reset types: %v)", suppportedResetTypes)
	}

	err = bmc.SystemPowerOff()
	if err != nil {
		glog.V(100).Infof("Failed to perform ForceOff system reset: %v", err)

		return fmt.Errorf("failed to perform ForceOff system reset: %w", err)
	}

	glog.V(100).Infof("Waiting for system to be in power state %v", redfish.OffPowerState)

	// First, make sure the system is off.
	err = wait.PollUntilContextTimeout(context.TODO(),
		1*time.Second,
		5*time.Second,
		true,
		func(ctx context.Context) (bool, error) {
			powerState, err := bmc.SystemPowerState()
			if err != nil {
				glog.V(100).Infof("Failed to get system's power state: %v", err)

				return false, fmt.Errorf("failed to get system's power state: %w", err)
			}

			glog.V(100).Infof("System's current power state: %v", powerState)

			if powerState == string(redfish.OffPowerState) {
				return true, nil
			}

			// Wait and get power state again.
			return false, nil
		})
	if err != nil {
		glog.V(100).Infof("Failure waiting for system's power state to be %v: %v", redfish.OffPowerState, err)

		return fmt.Errorf("failure waiting for system's power state to be %v: %w", redfish.OffPowerState, err)
	}

	return bmc.SystemPowerOn()
}

// SystemPowerState returns the system's current power state using the Redfish API.
// Returned string can be one of On/Off/Paused/PoweringOn/PoweringOff.
func (bmc *BMC) SystemPowerState() (string, error) {
	if valid, err := bmc.validateRedfish(); !valid {
		return "", err
	}

	glog.V(100).Info("Collecting current power state from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return "", fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return "", fmt.Errorf("failed to get redfish system: %w", err)
	}

	return string(system.PowerState), nil
}

// WaitForSystemPowerState waits up to timeout until the BMC returns the provided system power state.
func (bmc *BMC) WaitForSystemPowerState(powerState redfish.PowerState, timeout time.Duration) error {
	if valid, err := bmc.validateRedfish(); !valid {
		return err
	}

	glog.V(100).Infof("Waiting up to %s until BMC returns power state %s", timeout, powerState)

	return wait.PollUntilContextTimeout(
		context.TODO(), 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			systemPowerState, err := bmc.SystemPowerState()
			if err != nil {
				glog.V(100).Infof("Failed to get system power state from BMC: %v", err)

				return false, nil
			}

			return systemPowerState == string(powerState), nil
		})
}

// PowerUsage returns the current power usage of the chassis in watts using the Redfish API. This method uses the first
// chassis with a power link and the power control index for the BMC client.
func (bmc *BMC) PowerUsage() (float32, error) {
	if valid, err := bmc.validateRedfish(); !valid {
		return 0.0, err
	}

	glog.V(100).Info("Collecting current power usage from bmc's redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return 0.0, fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	powerControl, err := redfishGetPowerControl(redfishClient, bmc.powerControlIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish power control: %v", err)

		return 0.0, fmt.Errorf("failed to get redfish power control: %w", err)
	}

	return powerControl.PowerConsumedWatts, nil
}

// SystemBootOptions uses the redfish api to get the current system's boot options and
// returns a map references to display names, e.g.:
//   - "Boot0000":"PXE Device 1: Embedded NIC 1 Port 1 Partition 1"
//   - "Boot0003":"RAID Controller in SL 3: Red Hat Enterprise Linux]"
func (bmc *BMC) SystemBootOptions() (map[string]string, error) {
	glog.V(100).Infof("Getting available boot options from redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return nil, fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return nil, fmt.Errorf("failed to get redfish system: %w", err)
	}

	bootOptions, err := system.BootOptions()
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system's boot options: %v", err)

		return nil, fmt.Errorf("failed to get redfish system's boot options: %w", err)
	}

	bootOptionsMap := map[string]string{}

	for _, bootOrderRef := range system.Boot.BootOrder {
		for _, bootOption := range bootOptions {
			if bootOrderRef == bootOption.BootOptionReference {
				bootOptionsMap[bootOrderRef] = bootOption.DisplayName
			}
		}
	}

	return bootOptionsMap, nil
}

// SystemBootOrderReferences returns the current system's boot order (references) in an ordered slice
// using redfish API.
func (bmc *BMC) SystemBootOrderReferences() ([]string, error) {
	glog.V(100).Infof("Getting BootOrder references from redfish endpoint")

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return nil, fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return nil, fmt.Errorf("failed to get redfish system: %w", err)
	}

	return system.Boot.BootOrder, nil
}

// SetSystemBootOrderReferences sets the boot order references of the current system using
// the redfish API. The boot order references are not updated until the system is resetted, meaning
// that a following call to SystemBootOrderReferences() won't reflect the change until the system has
// been actually resetted.
func (bmc *BMC) SetSystemBootOrderReferences(bootOrderReferences []string) error {
	glog.V(100).Infof("Setting BootOrder references (%+v) from redfish endpoint", bootOrderReferences)

	if len(bootOrderReferences) == 0 {
		glog.V(100).Infof("bootOrderReferences param cannot be empty")

		return fmt.Errorf("bootOrderReferences param cannot be empty")
	}

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return fmt.Errorf("failed to get redfish system: %w", err)
	}

	newBoot := redfish.Boot{
		BootOrder: bootOrderReferences,
	}

	glog.V(100).Infof("Setting new Boot value: %+v", newBoot)

	return system.SetBoot(newBoot)
}

// BootFromCD inserts the image available in isoUrl in the virtual media with virtualMediaID
// and boots from it only once.
func (bmc *BMC) BootFromCD(isoURL, virtualMediaID string) error {
	glog.V(100).Infof("Setting to boot from CD (ISO: %s)", isoURL)

	if len(isoURL) == 0 {
		glog.V(100).Infof("isoUrl param cannot be empty")

		return fmt.Errorf("isoUrl param cannot be empty")
	}

	if len(virtualMediaID) == 0 {
		glog.V(100).Infof("virtualMediaID param cannot be empty")

		return fmt.Errorf("virtualMediaID param cannot be empty")
	}

	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return fmt.Errorf("failed to get redfish system: %w", err)
	}

	glog.V(100).Infof("Setting virtual media: %+v", isoURL)

	virtualMedia, err := system.VirtualMedia()
	if err != nil {
		glog.V(100).Infof("Failed to retrieve virtual media: %v", err)
	}

	var cdrom *redfish.VirtualMedia

	for _, vm := range virtualMedia {
		if vm.MediaTypes != nil && vm.ID == virtualMediaID {
			for _, item := range vm.MediaTypes {
				if item == "CD" {
					cdrom = vm
				}
			}
		}
	}

	if cdrom == nil {
		glog.V(100).Infof("No CD virtual media slot found")

		return fmt.Errorf("no cd virtual media slot found")
	}

	err = cdrom.InsertMedia(isoURL, true, true)
	if err != nil {
		glog.V(100).Infof("Failed to insert virtual media: %v", err)

		return err
	}

	newBoot := redfish.Boot{
		BootSourceOverrideEnabled: redfish.OnceBootSourceOverrideEnabled,
		BootSourceOverrideTarget:  redfish.CdBootSourceOverrideTarget,
	}

	glog.V(100).Infof("Setting new Boot value: %+v", newBoot)

	err = system.SetBoot(newBoot)

	return err
}

// RunCLICommand runs a CLI command in the BMC's console over SSH. This method will block until the command has
// finished, and its output is copied to stdout and/or stderr if applicable. If combineOutput is true, stderr content is
// merged in stdout. The timeout param is used to avoid the caller to be stuck forever in case something goes wrong or
// the command is stuck.
func (bmc *BMC) RunCLICommand(
	cmd string, combineOutput bool, timeout time.Duration) (stdout string, stderr string, err error) {
	if valid, err := bmc.validateSSH(); !valid {
		return "", "", err
	}

	glog.V(100).Infof("Running CLI command in BMC's CLI: %s", cmd)

	client, err := bmc.createCLISSHClient()
	if err != nil {
		glog.V(100).Infof("Failed to connect to CLI: %v", err)

		return "", "", fmt.Errorf("failed to connect to CLI: %w", err)
	}
	// Create a session
	sshSession, err := client.NewSession()
	if err != nil {
		glog.V(100).Infof("Failed to create a new SSH session: %v", err)

		return "", "", fmt.Errorf("failed to create a new ssh session: %w", err)
	}

	defer client.Close()

	var stdoutBuffer, stderrBuffer bytes.Buffer
	if !combineOutput {
		sshSession.Stdout = &stdoutBuffer
		sshSession.Stderr = &stderrBuffer
	}

	var combinedOutput []byte

	errCh := make(chan error)

	go func() {
		var err error
		if combineOutput {
			combinedOutput, err = sshSession.CombinedOutput(cmd)
		} else {
			err = sshSession.Run(cmd)
		}

		errCh <- err
	}()

	timeoutCh := time.After(timeout)

	select {
	case <-timeoutCh:
		glog.V(100).Info("CLI command timeout")

		return stdoutBuffer.String(), stderrBuffer.String(), fmt.Errorf("timeout running command")
	case err := <-errCh:
		if err != nil {
			glog.V(100).Infof("Command run error: %v", err)

			return stdoutBuffer.String(), stderrBuffer.String(), fmt.Errorf("command run error: %w", err)
		}
	}

	if combineOutput {
		return string(combinedOutput), "", nil
	}

	return stdoutBuffer.String(), stderrBuffer.String(), nil
}

// OpenSerialConsole opens the serial console port. The console is tunneled in an underlying (CLI) ssh session that is
// opened in the BMC's ssh server. If openConsoleCliCmd is provided, it will be sent to the BMC's cli. Otherwise, a best
// effort will be made to run the appropriate cli command based on the system manufacturer. This method requires both a
// Redfish and SSH user configured.
//
//nolint:funlen
func (bmc *BMC) OpenSerialConsole(openConsoleCliCmd string) (io.Reader, io.WriteCloser, error) {
	// We use both Redfish and SSH so make sure both are valid before continuing.
	if valid, err := bmc.validateRedfish(); !valid {
		return nil, nil, err
	}

	if valid, err := bmc.validateSSH(); !valid {
		return nil, nil, err
	}

	glog.V(100).Infof("Opening serial console on %v.", bmc.host)

	if bmc.sshClientForSerialConsole != nil {
		glog.V(100).Infof("There is already a serial console opened for %v's BMC. Use OpenSerialConsole() first.",
			bmc.host)

		return nil, nil, fmt.Errorf("there is already a serial console opened for %v's BMC", bmc.host)
	}

	if openConsoleCliCmd == "" {
		// no cli command to get console port was provided, try to guess based on
		// manufacturer.
		manufacturer, err := bmc.SystemManufacturer()
		if err != nil {
			glog.V(100).Infof("Failed to get redifsh system manufacturer for %v: %v", bmc.host, err)

			return nil, nil, fmt.Errorf("failed to get redfish system manufacturer for %v: %w", bmc.host, err)
		}

		var found bool
		if openConsoleCliCmd, found = cliCmdSerialConsole[manufacturer]; !found {
			glog.V(100).Infof("CLI command to get serial console not found for manufacturer for %v: %v",
				bmc.host, manufacturer)

			return nil, nil, fmt.Errorf("cli command to get serial console not found for manufacturer for %v: %v",
				bmc.host, manufacturer)
		}
	}

	client, err := bmc.createCLISSHClient()
	if err != nil {
		glog.V(100).Infof("Failed to create underlying ssh session for %v: %v", bmc.host, err)

		return nil, nil, fmt.Errorf("failed to create underlying ssh session for %v: %w", bmc.host, err)
	}

	// Create a session
	sshSession, err := client.NewSession()
	if err != nil {
		glog.V(100).Infof("Failed to create a new SSH session: %v", err)

		return nil, nil, fmt.Errorf("failed to create a new ssh session: %w", err)
	}

	// Pipes need to be retrieved before session.Start()
	reader, err := sshSession.StdoutPipe()
	if err != nil {
		glog.V(100).Infof("Failed to get stdout pipe from %v's ssh session: %v", bmc.host, err)

		_ = client.Close()

		return nil, nil, fmt.Errorf("failed to get stdout pipe from %v's ssh session: %w", bmc.host, err)
	}

	writer, err := sshSession.StdinPipe()
	if err != nil {
		glog.V(100).Infof("Failed to get stdin pipe from from %v's ssh session: %w", bmc.host, err)

		_ = client.Close()

		return nil, nil, fmt.Errorf("failed to get stdin pipe from %v's ssh session: %w", bmc.host, err)
	}

	err = sshSession.Start(openConsoleCliCmd)
	if err != nil {
		glog.V(100).Infof("Failed to start CLI command %q on %v: %v", openConsoleCliCmd, bmc.host, err)

		_ = client.Close()

		return nil, nil, fmt.Errorf(
			"failed to start serial console with cli command %q on %v: %w", openConsoleCliCmd, bmc.host, err)
	}

	bmc.sshClientForSerialConsole = client

	return reader, writer, nil
}

// CloseSerialConsole closes the serial console's underlying ssh session.
func (bmc *BMC) CloseSerialConsole() error {
	if valid, err := bmc.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Closing serial console for %v.", bmc.host)

	if bmc.sshClientForSerialConsole == nil {
		glog.V(100).Infof("No underlying ssh session found for %v. Please use OpenSerialConsole() first.", bmc.host)

		return fmt.Errorf("no underlying ssh session found for %v", bmc.host)
	}

	err := bmc.sshClientForSerialConsole.Close()
	if err != nil {
		glog.V(100).Infof("Failed to close underlying ssh session for %v: %v", bmc.host, err)

		return fmt.Errorf("failed to close underlying ssh session for %v: %w", bmc.host, err)
	}

	bmc.sshClientForSerialConsole = nil

	return nil
}

// redfishConnect uses the provided host, credentials, and timeout to produce a gofish APIClient for accessing the
// Redfish API.
func redfishConnect(
	host, user, password string, sessionTimeout time.Duration) (*gofish.APIClient, context.CancelFunc, error) {
	gofishConfig := gofish.ClientConfig{
		Endpoint: "https://" + host,
		Username: user,
		Password: password,
		Insecure: true,
	}

	ctx, cancel := context.WithTimeout(context.TODO(), sessionTimeout)

	client, err := gofish.ConnectContext(ctx, gofishConfig)
	if err != nil {
		cancel()

		return nil, nil, fmt.Errorf("failed to connect to redfish endpoint: %w", err)
	}

	return client, cancel, nil
}

// redfishGetSystem uses the provided gofish APIClient and the system index to get a system from the Redfish API.
func redfishGetSystem(redfishClient *gofish.APIClient, index int) (*redfish.ComputerSystem, error) {
	systems, err := redfishClient.GetService().Systems()
	if err != nil {
		return nil, fmt.Errorf("failed to get systems: %w", err)
	}

	if len(systems) < index+1 {
		return nil, fmt.Errorf("invalid system index %d (base-index=0, num systems=%d)", index, len(systems))
	}

	return systems[index], nil
}

// redfishGetSystemSecureBoot uses the provided gofish APIClient and the system index to get the SecureBoot resource for
// a system.
func redfishGetSystemSecureBoot(redfishClient *gofish.APIClient, systemIndex int) (*redfish.SecureBoot, error) {
	system, err := redfishGetSystem(redfishClient, systemIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to get redfish system: %w", err)
	}

	sboot, err := system.SecureBoot()
	if err != nil {
		return nil, err
	}

	return sboot, nil
}

// redfishGetPowerControl gets the specified PowerControl from the first chassis with a power link from the redfish API.
func redfishGetPowerControl(
	redfishClient *gofish.APIClient, powerControlIndex int) (*redfish.PowerControl, error) {
	chassisCollection, err := redfishClient.GetService().Chassis()
	if err != nil {
		return nil, fmt.Errorf("failed to get chassis collection: %w", err)
	}

	for chassisIndex, chassis := range chassisCollection {
		power, err := chassis.Power()
		if err != nil {
			return nil, fmt.Errorf("failed to get power for chassis index %d: %w", chassisIndex, err)
		}

		if power == nil {
			continue
		}

		if powerControlIndex >= len(power.PowerControl) {
			return nil, fmt.Errorf(
				"invalid power control index %d (base-index=0, num power control=%d)", powerControlIndex, len(power.PowerControl))
		}

		return &power.PowerControl[powerControlIndex], nil
	}

	return nil, fmt.Errorf("failed to get power control: no chassis with power link found")
}

// validateRedfish performs the same validations as in validate but also checks for a valid redfish user.
func (bmc *BMC) validateRedfish() (bool, error) {
	if valid, err := bmc.validate(); !valid {
		return false, err
	}

	if bmc.redfishUser == nil {
		glog.V(100).Info("The BMC's Redfish user is nil")

		return false, fmt.Errorf("cannot access redfish with nil user")
	}

	return true, nil
}

func (bmc *BMC) validateSSH() (bool, error) {
	if valid, err := bmc.validate(); !valid {
		return false, err
	}

	if bmc.sshUser == nil {
		glog.V(100).Info("The BMC's SSH user is nil")

		return false, fmt.Errorf("cannot access ssh with nil user")
	}

	return true, nil
}

// validate checks that the BMC is in a valid state with no error message.
func (bmc *BMC) validate() (bool, error) {
	if bmc == nil {
		glog.V(100).Info("The BMC is nil")

		return false, fmt.Errorf("error: received nil bmc")
	}

	if bmc.errorMsg != "" {
		glog.V(100).Infof("The BMC has an error message: %s", bmc.errorMsg)

		return false, fmt.Errorf("%s", bmc.errorMsg)
	}

	return true, nil
}

func isResetTypeSupported(resetType redfish.ResetType, supportedTypes []redfish.ResetType) bool {
	for _, supportedType := range supportedTypes {
		if supportedType == resetType {
			return true
		}
	}

	return false
}

func (bmc *BMC) getSupportedResetTypes() ([]redfish.ResetType, error) {
	redfishClient, cancel, err := redfishConnect(
		bmc.host,
		bmc.redfishUser.Name,
		bmc.redfishUser.Password,
		bmc.timeOuts.Redfish)
	if err != nil {
		glog.V(100).Infof("Redfish connection error: %v", err)

		return nil, fmt.Errorf("redfish connection error: %w", err)
	}

	defer func() {
		redfishClient.Logout()
		cancel()
	}()

	system, err := redfishGetSystem(redfishClient, bmc.systemIndex)
	if err != nil {
		glog.V(100).Infof("Failed to get redfish system: %v", err)

		return nil, fmt.Errorf("failed to get redfish system: %w", err)
	}

	return system.SupportedResetTypes, nil
}

// createCLISSHClient creates a ssh Session to the host.
func (bmc *BMC) createCLISSHClient() (*ssh.Client, error) {
	if valid, err := bmc.validateSSH(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Creating SSH session to run commands in the BMC's CLI.")

	config := &ssh.ClientConfig{
		User: bmc.sshUser.Name,
		Auth: []ssh.AuthMethod{
			ssh.Password(bmc.sshUser.Password),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string,
				echos []bool) (answers []string, err error) {
				answers = make([]string, len(questions))
				// The second parameter is unused
				for n := range questions {
					answers[n] = bmc.sshUser.Password
				}

				return answers, nil
			}),
		},
		Timeout:         bmc.timeOuts.SSH,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	// Establish SSH connection
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", bmc.host, bmc.sshPort), config)
	if err != nil {
		glog.V(100).Infof("Failed to connect to BMC's SSH server: %v", err)

		return nil, fmt.Errorf("failed to connect to BMC's SSH server: %w", err)
	}

	return client, nil
}
