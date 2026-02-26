// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
//
// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package kacv1

import (
	corev1 "k8s.io/api/core/v1"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// GlobalValues defines the global values
// +k8s:openapi-gen=true
type GlobalValues struct {
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	ImagePullSecret string            `json:"imagePullSecret,omitempty"`
	ImageOverrides  map[string]string `json:"imageOverrides,omitempty"`
	NodeSelector    map[string]string `json:"nodeSelector,omitempty"`
	ProxyConfig     map[string]string `json:"proxyConfig,omitempty"`
}

const (
	HTTPProxy  = "HTTP_PROXY"
	HTTPSProxy = "HTTPS_PROXY"
	NoProxy    = "NO_PROXY"
)

// AddonAgentConfig is the configurations for addon agents.
type AddonAgentConfig struct {
	KlusterletAddonConfig    *KlusterletAddonConfig
	ManagedCluster           *clusterv1.ManagedCluster
	NodeSelector             map[string]string
	ImagePullSecret          string
	ImagePullSecretNamespace string
	ImagePullPolicy          corev1.PullPolicy
}

const (
	// UpgradeLabel is to label the upgraded manifestWork.
	UpgradeLabel = "open-cluster-management.io/upgrade"

	KlusterletAddonNamespace = "open-cluster-management-agent-addon"
)

const (
	WorkManagerAddonName     = "work-manager"
	ApplicationAddonName     = "application-manager"
	CertPolicyAddonName      = "cert-policy-controller"
	ConfigPolicyAddonName    = "config-policy-controller"
	IamPolicyAddonName       = "iam-policy-controller" // deprecated and removed
	PolicyAddonName          = "policy-controller"
	PolicyFrameworkAddonName = "governance-policy-framework"
	SearchAddonName          = "search-collector"
)

// KlusterletAddons is a list of managedClusterAddons which can be updated by addon-controller.
// true means it is deployed by addon-controller, can be updated and deleted.
// false means it is not deployed by addon-controller, do not need to be updated, can be deleted.
var KlusterletAddons = map[string]bool{
	WorkManagerAddonName:     false,
	ApplicationAddonName:     true,
	ConfigPolicyAddonName:    true,
	IamPolicyAddonName:       false, // deprecated and removed
	CertPolicyAddonName:      true,
	PolicyFrameworkAddonName: true,
	SearchAddonName:          true,
}

// KlusterletAddonImageNames is the image key names for each addon agents in image-manifest configmap
var KlusterletAddonImageNames = map[string][]string{
	ApplicationAddonName:     {"multicluster_operators_subscription"},
	ConfigPolicyAddonName:    {"config_policy_controller", "kube_rbac_proxy"},
	CertPolicyAddonName:      {"cert_policy_controller"},
	PolicyAddonName:          {"config_policy_controller", "governance_policy_framework_addon"},
	PolicyFrameworkAddonName: {"governance_policy_framework_addon", "kube_rbac_proxy"},
	SearchAddonName:          {"search_collector"},
}

// image env names
const (
	EnvMulticlusterOperatorSubscription = "MULTICLUSTER_OPERATOR_SUBSCRIPTION"
	EnvConfigPolicyController           = "CONFIG_POLICY_CONTROLLER"
	EnvKubeRBACProxy                    = "KUBE_RBAC_PROXY"
	EnvCertPolicyController             = "CERT_POLICY_CONTROLLER"
	EnvGovernancePolicyFrameworkAddon   = "GOVERNANCE_POLICY_FRAMEWORK_ADDON"
	EnvSearchCollector                  = "SEARCH_COLLECTOR"
)

// EnvImageNameMap is the image env names map
var EnvImageNameMap = map[string]string{
	EnvMulticlusterOperatorSubscription: "multicluster_operators_subscription",
	EnvKubeRBACProxy:                    "kube_rbac_proxy",
	EnvCertPolicyController:             "cert_policy_controller",
	EnvConfigPolicyController:           "config_policy_controller",
	EnvGovernancePolicyFrameworkAddon:   "governance_policy_framework_addon",
	EnvSearchCollector:                  "search_collector",
}
