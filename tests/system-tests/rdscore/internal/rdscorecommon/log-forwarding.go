package rdscorecommon

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	lokiv1 "github.com/grafana/loki/operator/apis/loki/v1"
	observabilityv1 "github.com/openshift/cluster-logging-operator/api/observability/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterlogging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/dns"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/route"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	kcatDeploymentName      = "kcat"
	kcatDeploymentNamespace = "default"
	logMessageCnt           = 2000
	lokiRouteName           = "logging-loki"
	lokiValidationNamespace = "loki-validation-test"
	lokiValidationSAName    = "loki-test-sa"
	lokiValidationCRBName   = "loki-test-infra-binding"
)

var (
	logTypes = []string{"audit", "infrastructure"}
)

type kafkaRecord struct {
	GeneralTimestamp string `json:"@timestamp"`
	Annotations      struct {
		Decision string `json:"authorization.k8s.io/decision,omitempty"`
		Reason   string `json:"authorization.k8s.io/reason,omitempty"`
	} `json:"annotations,omitempty"`
	Hostname   string `json:"hostname"`
	Kubernetes struct {
		Annotations struct {
			PodNetwork    string `json:"k8s.ovn.org/pod-networks,omitempty"`
			NetworkStatus string `json:"k8s.v1.cni.cncf.io/network-status,omitempty"`
			SCC           string `json:"openshift.io/scc,omitempty"`
		} `json:"annotations,omitempty"`
		ContainerID       string `json:"container_id,omitempty"`
		ContainerImage    string `json:"container_image,omitempty"`
		ContainerImageID  string `json:"container_image_id,omitempty"`
		ContainerIOStream string `json:"container_iostream,omitempty"`
		ContainerName     string `json:"container_name,omitempty"`
		Labels            struct {
			App             string `json:"app,omitempty"`
			PodTemplateHash string `json:"pod-template-hash,omitempty"`
		} `json:"labels,omitempty"`
		NamespaceID     string `json:"namespace_id,omitempty"`
		NamespaceLabels struct {
		} `json:"namespace_labels,omitempty"`
		NamespaceName string `json:"namespace_name,omitempty"`
		PodID         string `json:"pod_id,omitempty"`
		PodIP         string `json:"pod_ip,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodOwner      string `json:"pod_owner,omitempty"`
	} `json:"kubernetes,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
	AuditID    string `json:"auditID,omitempty"`
	AuditLevel string `json:"k8s_audit_level,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Level      string `json:"level"`
	LogSource  string `json:"log_source"`
	LogType    string `json:"log_type"`
	Message    string `json:"message,omitempty"`
	ObjectRef  struct {
		APIGroup        string `json:"apiGroup,omitempty"`
		APIVersion      string `json:"apiVersion,omitempty"`
		Name            string `json:"name,omitempty"`
		Namespace       string `json:"namespace,omitempty"`
		Resource        string `json:"resource,omitempty"`
		ResourceVersion string `json:"resourceVersion,omitempty"`
		UID             string `json:"uid,omitempty"`
	} `json:"objectRef,omitempty"`
	Openshift struct {
		ClusterID string `json:"cluster_id,omitempty"`
		Labels    struct {
			RDS      string `json:"rds,omitempty"`
			SiteName string `json:"sitename"`
			SiteUUID string `json:"siteuuid"`
		} `json:"labels,omitempty"`
		Sequence int64 `json:"sequence,omitempty"`
	} `json:"openshift,omitempty"`
	RequestReceivedTimestamp string `json:"requestReceivedTimestamp,omitempty"`
	RequestURI               string `json:"requestURI,omitempty"`
	ResponseStatus           struct {
		Code     int `json:"code,omitempty"`
		Metadata struct {
		} `json:"metadata,omitempty"`
	} `json:"responseStatus,omitempty"`
	SourceIps      []string `json:"sourceIPs,omitempty"`
	Stage          string   `json:"stage,omitempty"`
	StageTimestamp string   `json:"stageTimestamp,omitempty"`
	Timestamp      string   `json:"timestamp,omitempty"`
	User           struct {
		Extra struct {
			CredentialID []string `json:"authentication.kubernetes.io/credential-id,omitempty"`
			NodeName     []string `json:"authentication.kubernetes.io/node-name,omitempty"`
			NodeUID      []string `json:"authentication.kubernetes.io/node-uid,omitempty"`
			PodName      []string `json:"authentication.kubernetes.io/pod-name,omitempty"`
			PodUID       []string `json:"authentication.kubernetes.io/pod-uid,omitempty"`
		} `json:"extra,omitempty"`
		Groups   []string `json:"groups,omitempty"`
		UID      string   `json:"uid,omitempty"`
		UserName string   `json:"userName,omitempty"`
	} `json:"user,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
	Verb      string `json:"verb,omitempty"`
}

// VerifyLogForwardingToKafka Verify cluster log forwarding to the Kafka aggregator.
//
//nolint:funlen
func VerifyLogForwardingToKafka() {
	By("Insure CLO deployed")

	var ctx SpecContext

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify CLO namespace %s defined", rdscoreparams.CLONamespace)

	err := apiobjectshelper.VerifyNamespaceExists(APIClient, rdscoreparams.CLONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		rdscoreparams.CLONamespace, err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verify CLO deployment %s defined in namespace %s",
		rdscoreparams.CLODeploymentName, rdscoreparams.CLONamespace)

	err = apiobjectshelper.VerifyOperatorDeployment(APIClient,
		rdscoreparams.CLOName,
		rdscoreparams.CLODeploymentName,
		rdscoreparams.CLONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			rdscoreparams.CLOName, rdscoreparams.CLONamespace, err))

	By("Retrieve kafka server URL")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Retrieve Kafka server URL from the ClusterLogForwarder")

	clusterLogForwarder, err := clusterlogging.PullClusterLogForwarder(
		APIClient, rdscoreparams.CLOInstanceName, rdscoreparams.CLONamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to retrieve ClusterLogForwarder %s from the namespace %q; %v",
		rdscoreparams.CLOInstanceName, rdscoreparams.CLONamespace, err))

	clfOutput := clusterLogForwarder.Object.Spec.Outputs
	Expect(len(clfOutput)).ToNot(Equal(0), fmt.Sprintf(
		"No collector defined in the ClusterLogForwarder %s from the namespace %q",
		rdscoreparams.CLOInstanceName, rdscoreparams.CLONamespace))

	var kafkaURL, kafkaUser string

	for _, collector := range clfOutput {
		if collector.Type == "kafka" {
			clfKafkaURL := collector.Kafka.URL

			klog.V(100).Infof("collector.URL: %s", clfKafkaURL)

			kafkaURL = strings.Split(clfKafkaURL, "/")[2]
			kafkaUser = strings.Split(clfKafkaURL, "/")[3]

			break
		}
	}

	kafkaLogsLabel := RDSCoreConfig.KafkaLogsLabel

	if kafkaLogsLabel == "" {
		By("Getting cluster domain")

		clusterDNS, err := dns.Pull(APIClient)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
			"Failed to retrieve clusterDNS object cluster from the namespace default; %v", err))

		clusterDomain := clusterDNS.Object.Spec.BaseDomain
		Expect(clusterDomain).ToNot(Equal(""), "cluster domain is empty")

		klog.V(100).Infof("DEBUG: clusterDomain: %s", clusterDomain)

		kafkaLogsLabel = clusterDomain
	}

	By("Build query request command")

	cmdToRun := []string{"/bin/sh", "-c", fmt.Sprintf("kcat -b %s -C -t %s -C -q -o end -c %d | grep %s",
		kafkaURL, kafkaUser, logMessageCnt, kafkaLogsLabel)}

	By("Retrieve kcat pod object")

	kcatPodObj, err := getPodObjectByNamePattern(APIClient, kcatDeploymentName, kcatDeploymentNamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve %s pod object from namespace %s: %v",
			kcatDeploymentName, kcatDeploymentNamespace, err))

	By("Retrieve logs forwarded to the kafka")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Execute command: %q", cmdToRun)

	var logMessages []kafkaRecord

	Eventually(func() bool {
		output, err := kcatPodObj.ExecCommand(cmdToRun, kcatPodObj.Object.Spec.Containers[0].Name)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Error running command from within a pod %q in namespace %q: %v",
				kcatPodObj.Definition.Name, kcatPodObj.Definition.Namespace, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Successfully executed command from within a pod %q in namespace %q",
			kcatPodObj.Definition.Name, kcatPodObj.Definition.Namespace)

		result := output.String()

		if result == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Empty result received from within a pod %q in namespace %q",
				kcatPodObj.Definition.Name, kcatPodObj.Definition.Namespace)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Analyse received logs:\n\t%v", result)

		result = strings.TrimSpace(result)

		for _, line := range strings.Split(result, "\n") {
			var logMessage kafkaRecord

			err = json.Unmarshal([]byte(line), &logMessage)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error unmarshalling kafka record %q: %v", line, err)

				break
			}

			logMessages = append(logMessages, logMessage)
		}

		if len(logMessages) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No log messages forwarded to the kafka %s found", kafkaURL)

			return false
		}

		for _, logType := range logTypes {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Verify %s type log messages were forwarded to the kafka server %s", logType, kafkaURL)

			messageCnt := 0

			for _, logMessage := range logMessages {
				if logMessage.LogType == logType {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found record of the %s type: %v",
						logType, logMessage)

					messageCnt++
				}
			}

			if messageCnt == 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"No log messages of %q type forwarded to the kafka %q were found", logType, kafkaURL)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d %s log messages forwarded to the kafka server",
				messageCnt, logType)
		}

		return true
	}).WithContext(ctx).WithPolling(3*time.Second).WithTimeout(6*time.Minute).Should(BeTrue(),
		"failed to find log messages forwarded to the kafka server")
}

// VerifyLokiPodsRunning verifies local Loki pods are running in openshift-logging namespace.
func VerifyLokiPodsRunning(ctx SpecContext) {
	By("Verify namespace for local Loki exists")

	err := apiobjectshelper.VerifyNamespaceExists(APIClient, rdscoreparams.CLONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		rdscoreparams.CLONamespace, err))

	By("Verify Loki pods are in Running phase")

	Eventually(func() bool {
		podsList, err := pod.List(APIClient, rdscoreparams.CLONamespace, metav1.ListOptions{})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods in %q namespace: %v",
				rdscoreparams.CLONamespace, err)

			return false
		}

		lokiPodCount := 0

		for _, podObj := range podsList {
			if !strings.Contains(podObj.Definition.Name, "logging-loki") {
				continue
			}

			lokiPodCount++

			if podObj.Object.Status.Phase != corev1.PodRunning {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Loki pod %q is in %q phase in namespace %q",
					podObj.Definition.Name,
					podObj.Object.Status.Phase,
					podObj.Definition.Namespace)

				return false
			}
		}

		if lokiPodCount == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"No Loki pods found in namespace %q", rdscoreparams.CLONamespace)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Detected %d Loki pod(s) in Running phase in namespace %q",
			lokiPodCount, rdscoreparams.CLONamespace)

		return true
	}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"failed to verify local Loki pods are running")
}

// VerifyLokiStackReady verifies LokiStack resources exist and are Ready.
func VerifyLokiStackReady(ctx SpecContext) {
	By("Verify LokiStack custom resources can be listed")

	err := APIClient.AttachScheme(lokiv1.AddToScheme)
	Expect(err).ToNot(HaveOccurred(), "failed to attach LokiStack scheme to API client")

	By("Verify at least one LokiStack exists and all are in Ready status")

	Eventually(func() bool {
		lokiStackList := &lokiv1.LokiStackList{}

		err := APIClient.Client.List(context.TODO(), lokiStackList)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list LokiStack resources: %v", err)

			return false
		}

		if len(lokiStackList.Items) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info("No LokiStack resources found across namespaces")

			return false
		}

		for _, stack := range lokiStackList.Items {
			isReady := false

			for _, condition := range stack.Status.Conditions {
				if condition.Type == rdscoreparams.ConditionTypeReadyString && condition.Status == metav1.ConditionTrue {
					isReady = true

					break
				}
			}

			if !isReady {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"LokiStack %q in namespace %q is not Ready. Conditions: %+v",
					stack.Name,
					stack.Namespace,
					stack.Status.Conditions)

				return false
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Detected %d LokiStack resource(s); all are Ready",
			len(lokiStackList.Items))

		return true
	}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"failed to verify LokiStack resources are Ready")
}

// VerifyLokiPVCsBound verifies PVCs exist in openshift-logging and are all Bound.
func VerifyLokiPVCsBound(ctx SpecContext) {
	By("Verify PVC resources exist in openshift-logging namespace and all are Bound")

	Eventually(func() bool {
		pvcList, err := storage.ListPVC(APIClient, rdscoreparams.CLONamespace, metav1.ListOptions{})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Failed to list PVCs in namespace %q: %v", rdscoreparams.CLONamespace, err)

			return false
		}

		if len(pvcList) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"No PVC resources found in namespace %q", rdscoreparams.CLONamespace)

			return false
		}

		for _, pvcObj := range pvcList {
			if pvcObj.Object.Status.Phase != corev1.ClaimBound {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"PVC %q in namespace %q is in %q phase",
					pvcObj.Definition.Name, pvcObj.Definition.Namespace, pvcObj.Object.Status.Phase)

				return false
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Detected %d PVC(s) in namespace %q; all are Bound",
			len(pvcList), rdscoreparams.CLONamespace)

		return true
	}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(5*time.Minute).Should(BeTrue(),
		"failed to verify Loki PVC resources are Bound")
}

// VerifyClusterLogForwarderLokiConfiguration validates CLF output/pipeline and status for LokiStack forwarding.
func VerifyClusterLogForwarderLokiConfiguration(ctx SpecContext) {
	By("Verify ClusterLogForwarder exists in openshift-logging namespace")

	clusterLogForwarder, err := clusterlogging.PullClusterLogForwarder(
		APIClient, rdscoreparams.CLOInstanceName, rdscoreparams.CLONamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to retrieve ClusterLogForwarder %q in namespace %q",
			rdscoreparams.CLOInstanceName, rdscoreparams.CLONamespace))

	By("Verify ClusterLogForwarder includes LokiStack output and pipeline for audit and infrastructure logs")

	lokiOutputName := ""

	for _, output := range clusterLogForwarder.Object.Spec.Outputs {
		if output.Type != observabilityv1.OutputTypeLokiStack || output.LokiStack == nil {
			continue
		}

		if output.LokiStack.Target.Name == "" || output.LokiStack.Target.Namespace == "" {
			continue
		}

		lokiOutputName = output.Name

		break
	}

	Expect(lokiOutputName).ToNot(BeEmpty(),
		"ClusterLogForwarder does not include a valid LokiStack output")

	pipelineName := ""

	for _, pipeline := range clusterLogForwarder.Object.Spec.Pipelines {
		if !sliceContains(pipeline.InputRefs, "audit") || !sliceContains(pipeline.InputRefs, "infrastructure") {
			continue
		}

		if !sliceContains(pipeline.OutputRefs, lokiOutputName) {
			continue
		}

		pipelineName = pipeline.Name

		break
	}

	Expect(pipelineName).ToNot(BeEmpty(),
		fmt.Sprintf("no pipeline forwards audit and infrastructure logs to output %q", lokiOutputName))

	By("Verify ClusterLogForwarder status conditions indicate valid and ready configuration")

	Expect(hasTrueCondition(clusterLogForwarder.Object.Status.Conditions, observabilityv1.ConditionTypeReady)).
		To(BeTrue(), "ClusterLogForwarder Ready condition is not True")
	Expect(hasTrueConditionSuffix(clusterLogForwarder.Object.Status.InputConditions, "ValidInput/audit")).
		To(BeTrue(), "ClusterLogForwarder audit input validation condition is not True")
	Expect(hasTrueConditionSuffix(clusterLogForwarder.Object.Status.InputConditions, "ValidInput/infrastructure")).
		To(BeTrue(), "ClusterLogForwarder infrastructure input validation condition is not True")
	Expect(hasTrueConditionSuffix(clusterLogForwarder.Object.Status.OutputConditions, fmt.Sprintf("ValidOutput/%s", lokiOutputName))).
		To(BeTrue(), fmt.Sprintf("ClusterLogForwarder output validation condition is not True for %q", lokiOutputName))
	Expect(hasTrueConditionSuffix(clusterLogForwarder.Object.Status.PipelineConditions, fmt.Sprintf("ValidPipeline/%s", pipelineName))).
		To(BeTrue(), fmt.Sprintf("ClusterLogForwarder pipeline validation condition is not True for %q", pipelineName))
}

// VerifyLokiDistributorLogsNoErrors ensures distributor logs do not contain ingestion/API errors.
func VerifyLokiDistributorLogsNoErrors(ctx SpecContext) {
	By("Verify Loki distributor pods are running")

	distributorPods, err := pod.List(APIClient, rdscoreparams.CLONamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=distributor",
	})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to list distributor pods in namespace %q", rdscoreparams.CLONamespace))
	Expect(len(distributorPods)).To(BeNumerically(">", 0), "no distributor pods found")

	for _, distributorPod := range distributorPods {
		Expect(distributorPod.Object.Status.Phase).To(Equal(corev1.PodRunning),
			fmt.Sprintf("distributor pod %q is not Running", distributorPod.Definition.Name))
	}

	By("Verify distributor logs do not contain known ingestion/API errors")

	errorMarkers := []string{"error", "failed", "429", "500"}
	logLookback := 20 * time.Minute

	for _, distributorPod := range distributorPods {
		containerName := distributorPod.Object.Spec.Containers[0].Name

		logOutput, err := distributorPod.GetLog(logLookback, containerName)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to get logs for distributor pod %q", distributorPod.Definition.Name))

		lowerLogOutput := strings.ToLower(logOutput)

		for _, marker := range errorMarkers {
			Expect(lowerLogOutput).ToNot(ContainSubstring(marker),
				fmt.Sprintf("distributor pod %q logs contain marker %q", distributorPod.Definition.Name, marker))
		}
	}
}

// VerifyLokiQueryWithServiceAccountToken validates Loki API query using ServiceAccount token.
func VerifyLokiQueryWithServiceAccountToken(ctx SpecContext) {
	By("Create namespace and ServiceAccount for Loki query validation")

	validationNS := namespace.NewBuilder(APIClient, lokiValidationNamespace)
	if !validationNS.Exists() {
		_, err := validationNS.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create namespace %q", lokiValidationNamespace))

		DeferCleanup(func() {
			_ = validationNS.DeleteAndWait(2 * time.Minute)
		})
	}

	validationSA := serviceaccount.NewBuilder(APIClient, lokiValidationSAName, lokiValidationNamespace)
	if !validationSA.Exists() {
		_, err := validationSA.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create ServiceAccount %q in namespace %q",
				lokiValidationSAName, lokiValidationNamespace))

		DeferCleanup(func() {
			_ = validationSA.Delete()
		})
	}

	By("Bind ServiceAccount to infrastructure log view role")

	validationSubject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      lokiValidationSAName,
		Namespace: lokiValidationNamespace,
	}

	validationCRB := rbac.NewClusterRoleBindingBuilder(
		APIClient, lokiValidationCRBName, "cluster-logging-infrastructure-view", validationSubject)

	if !validationCRB.Exists() {
		_, err := validationCRB.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to create ClusterRoleBinding %q", lokiValidationCRBName))

		DeferCleanup(func() {
			_ = validationCRB.Delete()
		})
	}

	By("Generate token from ServiceAccount and query Loki route")

	token, err := validationSA.CreateToken(15 * time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to create token for ServiceAccount %q", lokiValidationSAName))
	Expect(token).ToNot(BeEmpty(), "generated token is empty")

	lokiRoute, err := route.Pull(APIClient, lokiRouteName, rdscoreparams.CLONamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to get route %q in namespace %q", lokiRouteName, rdscoreparams.CLONamespace))
	Expect(lokiRoute.Object.Spec.Host).ToNot(BeEmpty(),
		fmt.Sprintf("route %q host is empty", lokiRouteName))

	queryParams := url.Values{}
	queryParams.Set("query", `{log_type="infrastructure"}`)

	queryURL := fmt.Sprintf(
		"https://%s/api/logs/v1/infrastructure/loki/api/v1/query_range?%s",
		lokiRoute.Object.Spec.Host,
		queryParams.Encode())

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to create Loki query HTTP request")

	request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
	}

	response, err := httpClient.Do(request)
	Expect(err).ToNot(HaveOccurred(), "failed to execute Loki query HTTP request")

	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	Expect(err).ToNot(HaveOccurred(), "failed to read Loki query response body")
	Expect(response.StatusCode).To(Equal(http.StatusOK),
		fmt.Sprintf("unexpected Loki query status code %d: %s", response.StatusCode, string(responseBody)))

	var queryResponse struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string            `json:"resultType"`
			Result     []json.RawMessage `json:"result"`
		} `json:"data"`
	}

	err = json.Unmarshal(responseBody, &queryResponse)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to parse Loki query response JSON: %s", string(responseBody)))
	Expect(queryResponse.Status).To(Equal("success"), "Loki query returned non-success status")
	Expect(queryResponse.Data.ResultType).To(Equal("streams"), "Loki query resultType is not streams")
	Expect(len(queryResponse.Data.Result)).To(BeNumerically(">", 0),
		"Loki query returned no infrastructure log streams")
}

// VerifyLokiTopologySpreadConstraintsNotDefined verifies Loki workloads do not define topology spread constraints.
func VerifyLokiTopologySpreadConstraintsNotDefined(ctx SpecContext) {
	By("Verify Loki deployments do not define topologySpreadConstraints")

	deploymentList, err := deployment.List(APIClient, rdscoreparams.CLONamespace, metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to list deployments in namespace %q", rdscoreparams.CLONamespace))

	for _, deploymentObj := range deploymentList {
		if !strings.Contains(deploymentObj.Definition.Name, "logging-loki") {
			continue
		}

		Expect(len(deploymentObj.Definition.Spec.Template.Spec.TopologySpreadConstraints)).To(Equal(0),
			fmt.Sprintf("deployment %q has unexpected topologySpreadConstraints", deploymentObj.Definition.Name))
	}

	By("Verify Loki statefulsets do not define topologySpreadConstraints")

	statefulSetList, err := statefulset.List(APIClient, rdscoreparams.CLONamespace, metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to list statefulsets in namespace %q", rdscoreparams.CLONamespace))

	for _, statefulSetObj := range statefulSetList {
		if !strings.Contains(statefulSetObj.Definition.Name, "logging-loki") {
			continue
		}

		Expect(len(statefulSetObj.Definition.Spec.Template.Spec.TopologySpreadConstraints)).To(Equal(0),
			fmt.Sprintf("statefulset %q has unexpected topologySpreadConstraints", statefulSetObj.Definition.Name))
	}
}

func hasTrueCondition(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

func hasTrueConditionSuffix(conditions []metav1.Condition, conditionTypeSuffix string) bool {
	for _, condition := range conditions {
		if condition.Status != metav1.ConditionTrue {
			continue
		}

		if condition.Type == conditionTypeSuffix || strings.HasSuffix(condition.Type, fmt.Sprintf("/%s", conditionTypeSuffix)) {
			return true
		}
	}

	return false
}

func sliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
