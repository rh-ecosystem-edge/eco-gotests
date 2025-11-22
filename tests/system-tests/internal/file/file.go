package file

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
)

// TouchFile touches the file passed in parameter.
func TouchFile(path string) error {
	cmdToExec := fmt.Sprintf("sudo touch %s", path)

	return cluster.ExecCmd(APIClient, SystemTestsTestConfig.ControlPlaneLabel, cmdToExec)
}

// DeleteFile deletes the file passed in parameter.
func DeleteFile(path string) error {
	cmdToExec := fmt.Sprintf("sudo rm -f %s", path)

	err := cluster.ExecCmd(APIClient, SystemTestsTestConfig.ControlPlaneLabel, cmdToExec)
	if err != nil {
		return fmt.Errorf("error deleting file %s, err=%w", path, err)
	}

	return nil
}
