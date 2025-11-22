package systemtestsscc

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/scc"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsinittools"
)

// AddPrivilegedSCCtoDefaultSA adds default service account in a namespace to the privileged SCC.
func AddPrivilegedSCCtoDefaultSA(nsName string) error {
	privSCCRequired := true
	sccUser := fmt.Sprintf("system:serviceaccount:%s:default", nsName)

	privSCC, err := scc.Pull(APIClient, "privileged")
	if err != nil {
		return err
	}

	for _, usr := range privSCC.Object.Users {
		if usr == sccUser {
			privSCCRequired = false
		}
	}

	if privSCCRequired {
		_, err = privSCC.WithUsers([]string{sccUser}).Update()
		if err != nil {
			return err
		}
	}

	return nil
}
