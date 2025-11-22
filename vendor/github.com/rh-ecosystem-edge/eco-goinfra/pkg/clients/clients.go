package clients

import (
	"fmt"
	"os"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	clientConfigV1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	v1security "github.com/openshift/client-go/security/clientset/versioned/typed/security/v1"

	apiExt "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	appsV1Client "k8s.io/client-go/kubernetes/typed/apps/v1"
	networkV1Client "k8s.io/client-go/kubernetes/typed/networking/v1"
	rbacV1Client "k8s.io/client-go/kubernetes/typed/rbac/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"

	configV1 "github.com/openshift/api/config/v1"
	imageregistryV1 "github.com/openshift/api/imageregistry/v1"
	routev1 "github.com/openshift/api/route/v1"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"k8s.io/client-go/kubernetes/scheme"
	coreV1Client "k8s.io/client-go/kubernetes/typed/core/v1"
	storageV1Client "k8s.io/client-go/kubernetes/typed/storage/v1"

	appsv1 "k8s.io/api/apps/v1"
	scalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8sFakeClient "k8s.io/client-go/kubernetes/fake"
	fakeRuntimeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/openshift/api/operator/v1"
	machinev1beta1client "github.com/openshift/client-go/machine/clientset/versioned/typed/machine/v1beta1"
	operatorv1alpha1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1alpha1"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	policyv1clientTyped "k8s.io/client-go/kubernetes/typed/policy/v1"
)

// Settings provides the struct to talk with relevant API.
type Settings struct {
	KubeconfigPath string
	K8sClient      kubernetes.Interface
	coreV1Client.CoreV1Interface
	clientConfigV1.ConfigV1Interface
	networkV1Client.NetworkingV1Interface
	appsV1Client.AppsV1Interface
	rbacV1Client.RbacV1Interface
	Config *rest.Config
	runtimeClient.Client
	v1security.SecurityV1Interface
	dynamic.Interface
	operatorv1alpha1.OperatorV1alpha1Interface
	machinev1beta1client.MachineV1beta1Interface
	storageV1Client.StorageV1Interface
	policyv1clientTyped.PolicyV1Interface
	scheme *runtime.Scheme
}

// SchemeAttacher represents a function that can modify the clients current schemes.
type SchemeAttacher func(*runtime.Scheme) error

// New returns a *Settings with the given kubeconfig.
func New(kubeconfig string) *Settings {
	var (
		config *rest.Config
		err    error
	)

	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	if kubeconfig != "" {
		klog.V(100).Infof("Loading kube client config from path %s", kubeconfig)

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		klog.V(100).Info("Using in-cluster kube client config")

		config, err = rest.InClusterConfig()
	}

	if err != nil {
		klog.V(100).Infof("Failed to load kubeconfig: %v", err)

		return nil
	}

	clientSet := &Settings{}
	clientSet.CoreV1Interface = coreV1Client.NewForConfigOrDie(config)
	clientSet.ConfigV1Interface = clientConfigV1.NewForConfigOrDie(config)
	clientSet.AppsV1Interface = appsV1Client.NewForConfigOrDie(config)
	clientSet.NetworkingV1Interface = networkV1Client.NewForConfigOrDie(config)
	clientSet.RbacV1Interface = rbacV1Client.NewForConfigOrDie(config)
	clientSet.Interface = dynamic.NewForConfigOrDie(config)
	clientSet.SecurityV1Interface = v1security.NewForConfigOrDie(config)
	clientSet.OperatorV1alpha1Interface = operatorv1alpha1.NewForConfigOrDie(config)
	clientSet.MachineV1beta1Interface = machinev1beta1client.NewForConfigOrDie(config)
	clientSet.StorageV1Interface = storageV1Client.NewForConfigOrDie(config)
	clientSet.PolicyV1Interface = policyv1clientTyped.NewForConfigOrDie(config)
	clientSet.K8sClient = kubernetes.NewForConfigOrDie(config)
	clientSet.Config = config

	clientSet.scheme = runtime.NewScheme()

	err = SetScheme(clientSet.scheme)
	if err != nil {
		klog.V(100).Info("Error to load apiClient scheme")

		return nil
	}

	clientSet.Client, err = runtimeClient.New(config, runtimeClient.Options{
		Scheme: clientSet.scheme,
	})
	if err != nil {
		klog.V(100).Info("Error to create apiClient")

		return nil
	}

	clientSet.KubeconfigPath = kubeconfig

	return clientSet
}

// SetScheme returns mutated apiClient's scheme.
func SetScheme(crScheme *runtime.Scheme) error {
	if err := scheme.AddToScheme(crScheme); err != nil {
		return err
	}

	if err := apiExt.AddToScheme(crScheme); err != nil {
		return err
	}

	if err := imageregistryV1.Install(crScheme); err != nil {
		return err
	}

	if err := configV1.Install(crScheme); err != nil {
		return err
	}

	if err := operatorv1.Install(crScheme); err != nil {
		return err
	}

	if err := agentInstallV1Beta1.AddToScheme(crScheme); err != nil {
		return err
	}

	if err := routev1.AddToScheme(crScheme); err != nil {
		return err
	}

	if err := policyv1.AddToScheme(crScheme); err != nil {
		return err
	}

	return nil
}

// GetAPIClient implements the cluster.APIClientGetter interface.
func (settings *Settings) GetAPIClient() (*Settings, error) {
	if settings == nil {
		klog.V(100).Info("APIClient is nil")

		return nil, fmt.Errorf("APIClient cannot be nil")
	}

	return settings, nil
}

// AttachScheme attaches a scheme to the client's current scheme.
func (settings *Settings) AttachScheme(attacher SchemeAttacher) error {
	if settings == nil {
		klog.V(100).Info("APIClient is nil")

		return fmt.Errorf("cannot add scheme to nil client")
	}

	err := attacher(settings.scheme)
	if err != nil {
		return err
	}

	return nil
}

// TestClientParams provides the struct to store the parameters for the test client.
type TestClientParams struct {
	K8sMockObjects  []runtime.Object
	GVK             []schema.GroupVersionKind
	SchemeAttachers []SchemeAttacher

	// Note: Add more fields below if/when needed.
}

// GetTestClients returns a fake clientset for testing.
func GetTestClients(tcp TestClientParams) *Settings {
	clientSet, testBuilder := GetModifiableTestClients(tcp)
	clientSet.Client = testBuilder.Build()

	return clientSet
}

// GetModifiableTestClients returns a fake clientset
// and a modifiable clientbuilder for testing.
//
//nolint:funlen,gocyclo
func GetModifiableTestClients(tcp TestClientParams) (*Settings, *fakeRuntimeClient.ClientBuilder) {
	clientSet := &Settings{}

	var k8sClientObjects, genericClientObjects []runtime.Object

	//nolint:varnamelen
	for _, v := range tcp.K8sMockObjects {
		// Based on what type of object is, populate certain object slices
		// with what is supported by a certain client.
		// Add more items below if/when needed.
		switch v.(type) {
		// K8s Client Objects
		case *corev1.ServiceAccount:
			k8sClientObjects = append(k8sClientObjects, v)
		case *rbacv1.ClusterRole:
			k8sClientObjects = append(k8sClientObjects, v)
		case *rbacv1.ClusterRoleBinding:
			k8sClientObjects = append(k8sClientObjects, v)
		case *rbacv1.Role:
			k8sClientObjects = append(k8sClientObjects, v)
		case *rbacv1.RoleBinding:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Pod:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Service:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Node:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Secret:
			k8sClientObjects = append(k8sClientObjects, v)
		case *appsv1.Deployment:
			k8sClientObjects = append(k8sClientObjects, v)
		case *appsv1.StatefulSet:
			k8sClientObjects = append(k8sClientObjects, v)
		case *appsv1.ReplicaSet:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.ResourceQuota:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.PersistentVolume:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.PersistentVolumeClaim:
			k8sClientObjects = append(k8sClientObjects, v)
		case *policyv1.PodDisruptionBudget:
			k8sClientObjects = append(k8sClientObjects, v)
		case *scalingv1.HorizontalPodAutoscaler:
			k8sClientObjects = append(k8sClientObjects, v)
		case *storagev1.StorageClass:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.ConfigMap:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Event:
			k8sClientObjects = append(k8sClientObjects, v)
		case *netv1.NetworkPolicy:
			k8sClientObjects = append(k8sClientObjects, v)
		case *appsv1.DaemonSet:
			k8sClientObjects = append(k8sClientObjects, v)
		case *corev1.Namespace:
			k8sClientObjects = append(k8sClientObjects, v)
		// Generic Client Objects
		case *operatorv1.KubeAPIServer:
			genericClientObjects = append(genericClientObjects, v)
		case *operatorv1.OpenShiftAPIServer:
			genericClientObjects = append(genericClientObjects, v)
		case *routev1.Route:
			genericClientObjects = append(genericClientObjects, v)
		case *configV1.Node:
			genericClientObjects = append(genericClientObjects, v)
		case *operatorv1.IngressController:
			genericClientObjects = append(genericClientObjects, v)
		case *operatorv1.Console:
			genericClientObjects = append(genericClientObjects, v)
		case *configV1.ClusterOperator:
			genericClientObjects = append(genericClientObjects, v)
		case *agentInstallV1Beta1.AgentServiceConfig:
			genericClientObjects = append(genericClientObjects, v)
		}
	}

	// Assign the fake clientset to the clientSet
	clientSet.K8sClient = k8sFakeClient.NewSimpleClientset(k8sClientObjects...)
	clientSet.CoreV1Interface = clientSet.K8sClient.CoreV1()
	clientSet.AppsV1Interface = clientSet.K8sClient.AppsV1()
	clientSet.NetworkingV1Interface = clientSet.K8sClient.NetworkingV1()
	clientSet.RbacV1Interface = clientSet.K8sClient.RbacV1()
	clientSet.StorageV1Interface = clientSet.K8sClient.StorageV1()
	clientSet.PolicyV1Interface = clientSet.K8sClient.PolicyV1()

	// Update the generic client with schemes of generic resources
	clientSet.scheme = runtime.NewScheme()

	err := SetScheme(clientSet.scheme)
	if err != nil {
		return nil, nil
	}

	if len(tcp.GVK) > 0 && len(genericClientObjects) > 0 {
		clientSet.scheme.AddKnownTypeWithName(
			tcp.GVK[0], genericClientObjects[0])
	}

	if len(tcp.K8sMockObjects) > 0 && len(tcp.SchemeAttachers) > 0 {
		genericClientObjects = append(genericClientObjects, tcp.K8sMockObjects...)
	} else {
		clientSet.Interface = dynamicFake.NewSimpleDynamicClient(clientSet.scheme, genericClientObjects...)
	}

	for _, attacher := range tcp.SchemeAttachers {
		err := clientSet.AttachScheme(attacher)
		if err != nil {
			return nil, nil
		}
	}
	// Add fake runtime client to clientSet runtime client
	clientBuilder := fakeRuntimeClient.NewClientBuilder().WithScheme(clientSet.scheme).
		WithRuntimeObjects(genericClientObjects...)

	return clientSet, clientBuilder
}
