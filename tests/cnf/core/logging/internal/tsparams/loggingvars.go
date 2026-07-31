package tsparams

import (
	"github.com/openshift-kni/k8sreporter"
	lokiv1 "github.com/grafana/loki/operator/apis/loki/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreparams"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = append(coreparams.Labels, LabelSuite)
	// ReporterNamespacesToDump tells to the reporter from where to collect logs.
	ReporterNamespacesToDump = map[string]string{
		LokiOperatorNamespace: LokiOperatorNamespace,
		LoggingNamespace:      LoggingNamespace,
	}
	// ReporterCRDsToDump tells to the reporter what CRs to dump.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &lokiv1.LokiStackList{}},
	}

	// LokiStackDeploymentNames lists the expected LokiStack component deployments.
	LokiStackDeploymentNames = []string{
		"logging-loki-distributor",
		"logging-loki-gateway",
		"logging-loki-querier",
		"logging-loki-query-frontend",
	}

	// LokiStackStatefulSetNames lists the expected LokiStack component statefulsets.
	LokiStackStatefulSetNames = []string{
		"logging-loki-compactor",
		"logging-loki-ingester",
		"logging-loki-index-gateway",
	}
)
