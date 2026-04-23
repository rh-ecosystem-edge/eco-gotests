package helpers

import (
	"fmt"
	"strings"

	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"sigs.k8s.io/yaml"
)

// ParseClusterInstance parses kustomize multi-doc YAML and returns the ClusterInstance CR.
func ParseClusterInstance(kustomizeOutput []byte) (*siteconfigv1alpha1.ClusterInstance, error) {
	docs := strings.Split(string(kustomizeOutput), "---")

	for _, doc := range docs {
		if strings.TrimSpace(doc) == "" {
			continue
		}

		var clusterInst siteconfigv1alpha1.ClusterInstance

		err := yaml.Unmarshal([]byte(doc), &clusterInst)
		if err != nil {
			continue
		}

		if clusterInst.Kind == "ClusterInstance" {
			return &clusterInst, nil
		}
	}

	return nil, fmt.Errorf("ClusterInstance not found in kustomize output")
}
