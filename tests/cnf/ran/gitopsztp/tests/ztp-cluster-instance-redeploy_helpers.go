package tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/client"
	alertmanageralert "github.com/prometheus/alertmanager/api/v2/client/alert"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	ocmclusterv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocm/clusterv1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
)

func redeployClustersApplicationName() string {
	if name := os.Getenv("ECO_CNF_RAN_GITOPS_CLUSTERS_APP_NAME"); name != "" {
		return name
	}

	return tsparams.ArgoCdClustersAppName
}

func newACMMetricsClient() (prometheusv1.API, error) {
	metricsRoute, err := route.Pull(HubAPIClient, "rbac-query-proxy", ranparam.ACMObservabilityNamespace)
	if err != nil {
		return nil, err
	}
	if len(metricsRoute.Definition.Status.Ingress) == 0 {
		return nil, fmt.Errorf("ACM metrics route has no ingress")
	}

	token, err := querier.GetQuerierToken(HubAPIClient)
	if err != nil {
		return nil, err
	}
	caPool, err := rancluster.GetClusterDefaultRouterCAPool(HubAPIClient)
	if err != nil {
		return nil, err
	}

	return querier.CreatePrometheusAPI(metricsRoute.Definition.Status.Ingress[0].Host, token, caPool)
}

func createOrUpdateRedeployProbeRule(testID, summary string) (*monitoringv1.PrometheusRule, error) {
	if err := Spoke1APIClient.AttachScheme(monitoringv1.AddToScheme); err != nil {
		return nil, err
	}

	rule := &monitoringv1.PrometheusRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "eco-gotests-redeploy-" + testID,
			Namespace: ranparam.OpenshiftMonitoringNamespace,
			Labels:    map[string]string{"prometheus": "k8s", "role": "alert-rules"},
		},
		Spec: monitoringv1.PrometheusRuleSpec{Groups: []monitoringv1.RuleGroup{{
			Name: "eco-gotests-cluster-redeploy",
			Rules: []monitoringv1.Rule{{
				Alert: "EcoGotestsClusterRedeployProbe",
				Expr:  intstr.FromString("vector(1)"),
				Labels: map[string]string{
					"severity": "warning", "eco_gotests_test_id": testID, "managed_cluster": RANConfig.Spoke1Name,
				},
				Annotations: map[string]string{"summary": summary},
			}},
		}}},
	}
	if err := Spoke1APIClient.Create(context.Background(), rule); err == nil {
		return rule, nil
	} else if !apierrors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create probe PrometheusRule %s/%s: %w", rule.Namespace, rule.Name, err)
	}

	key := types.NamespacedName{Namespace: rule.Namespace, Name: rule.Name}
	currentRule := &monitoringv1.PrometheusRule{}
	if err := Spoke1APIClient.Get(context.Background(), key, currentRule); err != nil {
		return nil, fmt.Errorf("get existing probe PrometheusRule %s/%s: %w", key.Namespace, key.Name, err)
	}
	currentRule.Labels = rule.Labels
	currentRule.Spec = rule.Spec
	if err := Spoke1APIClient.Update(context.Background(), currentRule); err != nil {
		return nil, fmt.Errorf("update existing probe PrometheusRule %s/%s: %w", key.Namespace, key.Name, err)
	}

	return currentRule, nil
}

func updateRedeployProbeRuleSummary(rule *monitoringv1.PrometheusRule, summary string) error {
	if rule == nil {
		return fmt.Errorf("probe PrometheusRule is nil")
	}

	key := types.NamespacedName{Namespace: rule.Namespace, Name: rule.Name}
	currentRule := &monitoringv1.PrometheusRule{}
	if err := Spoke1APIClient.Get(context.Background(), key, currentRule); err != nil {
		return fmt.Errorf("get probe PrometheusRule %s/%s: %w", key.Namespace, key.Name, err)
	}
	if len(currentRule.Spec.Groups) == 0 || len(currentRule.Spec.Groups[0].Rules) == 0 {
		return fmt.Errorf("probe PrometheusRule %s/%s has no alert rule", key.Namespace, key.Name)
	}

	probe := &currentRule.Spec.Groups[0].Rules[0]
	if probe.Annotations == nil {
		probe.Annotations = make(map[string]string)
	}
	probe.Annotations["summary"] = summary

	if err := Spoke1APIClient.Update(context.Background(), currentRule); err != nil {
		return fmt.Errorf("update probe PrometheusRule %s/%s: %w", key.Namespace, key.Name, err)
	}

	return nil
}

func readySpokeNodeCount() (int, error) {
	nodeList, err := nodes.List(Spoke1APIClient)
	if err != nil {
		return 0, err
	}
	if len(nodeList) == 0 {
		return 0, fmt.Errorf("spoke has no nodes")
	}

	for _, node := range nodeList {
		ready := false
		for _, condition := range node.Definition.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return 0, fmt.Errorf("spoke node %s is not Ready", node.Definition.Name)
		}
	}

	return len(nodeList), nil
}

func waitForClusterInstance(name string, namespace string, present bool, timeout time.Duration) error {
	if err := HubAPIClient.AttachScheme(siteconfigv1alpha1.AddToScheme); err != nil {
		return err
	}

	return waitForHubObject(
		&siteconfigv1alpha1.ClusterInstance{},
		types.NamespacedName{Name: name, Namespace: namespace}, present, timeout)
}

func waitForManagedCluster(name string, present bool, timeout time.Duration) error {
	if err := HubAPIClient.AttachScheme(ocmclusterv1.Install); err != nil {
		return err
	}

	return waitForHubObject(
		&ocmclusterv1.ManagedCluster{},
		types.NamespacedName{Name: name}, present, timeout)
}

func waitForHubObject(object goclient.Object, key types.NamespacedName, present bool, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.Background(), 5*time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			err := HubAPIClient.Get(ctx, key, object)
			if apierrors.IsNotFound(err) {
				return !present, nil
			}
			if err != nil {
				return false, err
			}

			return present, nil
		})
}

func syncClustersApplication(timeout time.Duration) error {
	app, err := argocd.PullApplication(
		HubAPIClient, redeployClustersApplicationName(), ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		return fmt.Errorf("get clusters Argo CD application: %w", err)
	}

	app.Definition.Operation = &argocdtypes.Operation{
		Sync: &argocdtypes.SyncOperation{Prune: false},
	}
	if _, err = app.Update(false); err != nil {
		return fmt.Errorf("request sync for clusters Argo CD application: %w", err)
	}
	if err = app.WaitForSourceUpdate(true, timeout); err != nil {
		return fmt.Errorf("wait for clusters Argo CD application to become Synced: %w", err)
	}
	if err = waitForClusterInstance(RANConfig.Spoke1Name, RANConfig.Spoke1Name, true, timeout); err != nil {
		return fmt.Errorf("wait for ClusterInstance %s/%s to be created: %w",
			RANConfig.Spoke1Name, RANConfig.Spoke1Name, err)
	}

	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	if err != nil {
		return fmt.Errorf("get ClusterInstance %s/%s: %w", RANConfig.Spoke1Name, RANConfig.Spoke1Name, err)
	}
	_, err = clusterInstance.WaitForCondition(metav1.Condition{
		Type: string(siteconfigv1alpha1.ClusterProvisioned), Reason: string(siteconfigv1alpha1.Completed),
	}, timeout)
	if err != nil {
		return fmt.Errorf("wait for ClusterInstance %s/%s to report ClusterProvisioned/Completed: %w",
			RANConfig.Spoke1Name, RANConfig.Spoke1Name, err)
	}
	if err = waitForManagedCluster(RANConfig.Spoke1Name, true, timeout); err != nil {
		return fmt.Errorf("wait for ManagedCluster %s to be created: %w", RANConfig.Spoke1Name, err)
	}

	managedCluster, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
	if err != nil {
		return fmt.Errorf("get ManagedCluster %s: %w", RANConfig.Spoke1Name, err)
	}
	_, err = managedCluster.WaitForCondition(metav1.Condition{
		Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue,
	}, timeout)

	if err != nil {
		return fmt.Errorf("wait for ManagedCluster %s to become Available: %w", RANConfig.Spoke1Name, err)
	}

	return nil
}

func refreshSpokeAPIClientFromClusterDeployment() error {
	clusterDeployment, err := hive.PullClusterDeployment(
		HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	if err != nil {
		return fmt.Errorf("get spoke ClusterDeployment: %w", err)
	}

	secretName := clusterDeployment.Object.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name
	if secretName == "" {
		return fmt.Errorf("spoke ClusterDeployment has no admin kubeconfig Secret reference")
	}

	kubeconfigSecret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: secretName, Namespace: clusterDeployment.Object.Namespace}
	if err = HubAPIClient.Get(context.Background(), secretKey, kubeconfigSecret); err != nil {
		return fmt.Errorf("get spoke admin kubeconfig Secret %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}

	kubeconfig, found := kubeconfigSecret.Data["kubeconfig"]
	if !found || len(kubeconfig) == 0 {
		return fmt.Errorf("spoke admin kubeconfig Secret %s/%s has no kubeconfig data", secretKey.Namespace, secretKey.Name)
	}

	kubeconfigPath := RANConfig.Spoke1Kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if err = replaceKubeconfigFile(kubeconfigPath, kubeconfig); err != nil {
		return fmt.Errorf("refresh spoke kubeconfig at %s: %w", kubeconfigPath, err)
	}

	spokeClient := clients.New(kubeconfigPath)
	if spokeClient == nil {
		return fmt.Errorf("create spoke API client from ClusterDeployment kubeconfig")
	}
	if err = spokeClient.AttachScheme(monitoringv1.AddToScheme); err != nil {
		return fmt.Errorf("attach PrometheusRule scheme to refreshed spoke API client: %w", err)
	}
	spokeVersion, err := version.GetOCPVersion(spokeClient)
	if err != nil {
		return fmt.Errorf("validate refreshed spoke API client: %w", err)
	}
	if err = SetSpoke1APIClient(spokeClient); err != nil {
		return fmt.Errorf("replace spoke API client: %w", err)
	}
	RANConfig.Spoke1OCPVersion = spokeVersion

	return nil
}

func replaceKubeconfigFile(path string, data []byte) error {
	paths := filepath.SplitList(path)
	if len(paths) != 1 || paths[0] == "" {
		return fmt.Errorf("expected one kubeconfig file path, got %q", path)
	}

	resolvedPath, err := filepath.EvalSymlinks(paths[0])
	if err != nil {
		return fmt.Errorf("resolve kubeconfig path: %w", err)
	}

	fileInfo, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("stat kubeconfig: %w", err)
	}
	fileOwner, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read kubeconfig file ownership")
	}

	temporaryFile, err := os.CreateTemp(filepath.Dir(resolvedPath), ".kubeconfig-*")
	if err != nil {
		return fmt.Errorf("create temporary kubeconfig: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	keepTemporaryFile := false
	defer func() {
		if !keepTemporaryFile {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err = temporaryFile.Write(data); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("write temporary kubeconfig: %w", err)
	}
	if err = temporaryFile.Chown(int(fileOwner.Uid), int(fileOwner.Gid)); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("preserve kubeconfig ownership: %w", err)
	}
	if err = temporaryFile.Chmod(fileInfo.Mode().Perm()); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("preserve kubeconfig permissions: %w", err)
	}
	if err = temporaryFile.Sync(); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("sync temporary kubeconfig: %w", err)
	}
	if err = temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary kubeconfig: %w", err)
	}
	if err = os.Rename(temporaryPath, resolvedPath); err != nil {
		return fmt.Errorf("replace kubeconfig: %w", err)
	}
	keepTemporaryFile = true

	return nil
}

func setClustersSyncPolicy(policy *argocdtypes.SyncPolicy) error {
	app, err := argocd.PullApplication(
		HubAPIClient, redeployClustersApplicationName(), ranparam.OpenshiftGitOpsNamespace)
	if err != nil {
		return err
	}

	if policy == nil {
		app.Definition.Spec.SyncPolicy = nil
	} else {
		app.Definition.Spec.SyncPolicy = policy.DeepCopy()
	}
	if _, err = app.Update(false); err != nil {
		return err
	}

	return app.WaitForSourceUpdate(true, 10*time.Minute)
}

func querierResourcesNeedCleanup() (bool, error) {
	if err := HubAPIClient.AttachScheme(corev1.AddToScheme); err != nil {
		return false, err
	}
	if err := HubAPIClient.AttachScheme(rbacv1.AddToScheme); err != nil {
		return false, err
	}

	serviceAccount := &corev1.ServiceAccount{}
	err := HubAPIClient.Get(context.Background(), types.NamespacedName{
		Name: ranparam.QuerierServiceAccountName, Namespace: ranparam.OpenshiftMonitoringNamespace,
	}, serviceAccount)
	serviceAccountMissing := apierrors.IsNotFound(err)
	if err != nil && !serviceAccountMissing {
		return false, err
	}

	roleBinding := &rbacv1.ClusterRoleBinding{}
	err = HubAPIClient.Get(context.Background(), types.NamespacedName{Name: ranparam.QuerierCRBName}, roleBinding)
	roleBindingMissing := apierrors.IsNotFound(err)
	if err != nil && !roleBindingMissing {
		return false, err
	}
	if serviceAccountMissing != roleBindingMissing {
		return false, fmt.Errorf("ACM metrics access has an incomplete querier ServiceAccount/ClusterRoleBinding setup")
	}

	return serviceAccountMissing, nil
}

func redeployACMSpokeMetricExists(api prometheusv1.API, spokeName string) (bool, error) {
	result, _, err := api.Query(context.Background(),
		fmt.Sprintf("cluster_infrastructure_provider{cluster=%q}", spokeName), time.Now())
	if err != nil {
		return false, err
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return false, fmt.Errorf("unexpected ACM metrics result %T", result)
	}

	return len(vector) > 0, nil
}

func redeployACMSpokeAlertExists(api *alertmanagerv2.AlertmanagerAPI, testID string) (bool, error) {
	params := alertmanageralert.NewGetAlertsParamsWithTimeout(30 * time.Second).
		WithFilter([]string{fmt.Sprintf("eco_gotests_test_id=%q", testID)})
	response, err := api.Alert.GetAlerts(params)
	if err != nil {
		return false, err
	}

	return len(response.Payload) > 0, nil
}

func redeployACMSpokeAlertHasSummary(
	api *alertmanagerv2.AlertmanagerAPI, testID, summary string) (bool, []string, error) {
	params := alertmanageralert.NewGetAlertsParamsWithTimeout(30 * time.Second).
		WithFilter([]string{fmt.Sprintf("eco_gotests_test_id=%q", testID)})
	response, err := api.Alert.GetAlerts(params)
	if err != nil {
		return false, nil, err
	}

	observedSummaries := make([]string, 0, len(response.Payload))
	for _, alert := range response.Payload {
		observedSummaries = append(observedSummaries, alert.Annotations["summary"])
		if alert.Annotations["summary"] == summary {
			return true, observedSummaries, nil
		}
	}

	return false, observedSummaries, nil
}
