package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"text/template"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	k8syaml "sigs.k8s.io/yaml"

	"github.com/go-logr/logr"
	ibuv1 "github.com/openshift-kni/lifecycle-agent/api/imagebasedupgrade/v1"
	"github.com/openshift-kni/lifecycle-agent/controllers/utils"
	"github.com/openshift-kni/lifecycle-agent/internal/common"
	cp "github.com/otiai10/copy"
	"github.com/sirupsen/logrus"
)

// MarshalToFile marshals anything and writes it to the given file path. file only readable by root
func MarshalToFile(data any, filePath string) error {
	marshaled, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshall data: %w", err)
	}
	err = os.WriteFile(filePath, marshaled, 0o600)
	if err != nil {
		return fmt.Errorf("failed to write file to %s: %w", filePath, err)
	}
	return nil
}

// MarshalToYamlFile marshals any object to YAML and writes it to the given file path
// file only readable by root
func MarshalToYamlFile(data any, filePath string) error {
	marshaled, err := k8syaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed marshall file to yaml %s: %w", filePath, err)
	}
	if err := os.WriteFile(filePath, marshaled, 0o600); err != nil {
		return fmt.Errorf("failed to write file in %s: %w", filePath, err)
	}
	return nil
}

// RenderTemplate render template
func RenderTemplate(templateData string, params any) ([]byte, error) {
	tmpl := template.New("template")
	tmpl = template.Must(tmpl.Parse(templateData))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return nil, fmt.Errorf("failed to render template: %w", err)
	}

	return buf.Bytes(), nil
}

func GetSNOMasterNode(ctx context.Context, client runtimeclient.Client) (*corev1.Node, error) {
	nodesList := &corev1.NodeList{}
	err := client.List(ctx, nodesList, &runtimeclient.ListOptions{LabelSelector: labels.SelectorFromSet(
		labels.Set{
			"node-role.kubernetes.io/master": "",
		},
	)})
	if err != nil {
		return nil, fmt.Errorf("failed list nodes: %w", err)
	}
	if len(nodesList.Items) != 1 {
		return nil, fmt.Errorf("we should have one master node in sno cluster, current number is %d", len(nodesList.Items))
	}
	return &nodesList.Items[0], nil
}

func ReadYamlOrJSONFile(fp string, into any) error {
	decoder, err := GetYamlOrJsonDecoder(fp)
	if err != nil {
		return err
	}
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("failed to decode %s: %w", fp, err)
	}
	return nil
}

func GetYamlOrJsonDecoder(fp string) (*yaml.YAMLOrJSONDecoder, error) {
	fp = filepath.Clean(fp)

	data, err := os.ReadFile(fp)
	if err != nil {
		return nil, err // nolint:wrapcheck
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	return decoder, nil
}

func IsIpv6(provideIp string) bool {
	ip := net.ParseIP(provideIp)
	if ip == nil {
		return false
	}
	return ip.To4() == nil
}

func CreateKubeClient(scheme *runtime.Scheme, kubeconfig string) (runtimeclient.Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from flags for kube client: %w", err)
	}
	config.WarningHandler = rest.NoWarnings{}
	rc, err := runtimeclient.New(config, runtimeclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create runtimeclient for kube: %w", err)
	}
	return rc, nil
}

func RunOnce(name, directory string, log *logrus.Logger, f any, args ...any) error {
	doneFile := path.Join(directory, name+".done")
	_, err := os.Stat(doneFile)
	if err == nil || !os.IsNotExist(err) {
		log.Info(fmt.Sprintf("%s already exists, skipping", doneFile))
		return nil
	}

	fValue := reflect.ValueOf(f)

	var fArgs []reflect.Value
	for _, arg := range args {
		fArgs = append(fArgs, reflect.ValueOf(arg))
	}

	resultValues := fValue.Call(fArgs)
	if len(resultValues) > 0 {
		errVal, ok := resultValues[0].Interface().(error)
		if ok {
			return errVal
		}
	}

	_, err = os.Create(doneFile)
	if err != nil {
		return fmt.Errorf("failed to create RunOnce file: %w", err)
	}

	return nil
}

func ReadImageFromStaticPodDefinition(podFile, containerName string) (string, error) {
	pod := &corev1.Pod{}
	if err := ReadYamlOrJSONFile(podFile, pod); err != nil {
		return "", fmt.Errorf("failed to read %s pod static file, err: %w", podFile, err)
	}

	var etcdImage string
	for _, container := range pod.Spec.Containers {
		if container.Name == containerName {
			etcdImage = container.Image
			return etcdImage, nil
		}
	}

	return "", fmt.Errorf("no '%s' container found or no image specified in %s", containerName, podFile)
}

func HandleFilesWithCallback(folder string, action func(string) error) error {
	return filepath.Walk(folder, func(path string, info os.FileInfo, err error) error { //nolint:wrapcheck
		if err != nil {
			return fmt.Errorf("failed to walk path %s: %w", path, err)
		}

		if info.IsDir() {
			return nil
		}

		return action(path)
	})
}

func CopyFileIfExists(source, dest string) error {
	return cp.Copy(source, dest, cp.Options{OnError: func(src, dest string, err error) error { //nolint:wrapcheck
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}})
}

// CopyToTempFile copies a file to a temporary file.
// WARNING: This function only preserves POSIX permissions to the new file
// If you want to use it, take that into account and extend it if needed to
// also preserve other things like owner, extended attributes, selinux contexts
// or whatever might be needed
func CopyToTempFile(sourceFileName, directory, pattern string) (string, error) {
	destinationFile, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer destinationFile.Close()
	destinationFileName := destinationFile.Name()

	sourceFile, err := os.Open(sourceFileName)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", sourceFileName, err)
	}
	defer sourceFile.Close()

	if _, err = io.Copy(destinationFile, sourceFile); err != nil {
		return "", fmt.Errorf("failed to copy %s to temporary file %s: %w", sourceFileName, destinationFileName, err)
	}

	// Preserve POSIX permissions
	sourceFileInfo, err := os.Stat(sourceFileName)
	if err != nil {
		return "", fmt.Errorf("failed to get permissions of file %s: %w", sourceFileName, err)
	}
	// Chmod
	err = destinationFile.Chmod(sourceFileInfo.Mode())
	if err != nil {
		return "", fmt.Errorf("failed to set permissions on file %s: %w", destinationFileName, err)
	}

	return destinationFileName, nil
}

func ReplaceImageRegistry(image, targetRegistry, sourceRegistry string) (string, error) {
	if sourceRegistry == "" || targetRegistry == "" || targetRegistry == sourceRegistry {
		return image, nil
	}

	re, err := regexp.Compile(fmt.Sprintf("^%s", sourceRegistry))
	if err != nil {
		return "", fmt.Errorf("failed to create regex for registry replacement, err: %w", err)
	}
	return re.ReplaceAllString(image, targetRegistry), nil
}

func RemoveListOfFolders(log *logrus.Logger, folders []string) error {
	for _, folder := range folders {
		log.Infof("Removing %s folder", folder)
		if err := os.RemoveAll(folder); err != nil {
			return fmt.Errorf("failed to remove %s folder: %w", folder, err)
		}
	}
	return nil
}

func InitIBU(ctx context.Context, c client.Client, log *logr.Logger) error {
	ibu := &ibuv1.ImageBasedUpgrade{}
	filePath := common.PathOutsideChroot(utils.IBUFilePath)
	if err := ReadYamlOrJSONFile(filePath, ibu); err != nil {
		if os.IsNotExist(err) {
			ibu = &ibuv1.ImageBasedUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: utils.IBUName,
				},
				Spec: ibuv1.ImageBasedUpgradeSpec{
					Stage: ibuv1.Stages.Idle,
				},
			}

			if err := c.Create(ctx, ibu); err != nil {
				if !k8serrors.IsAlreadyExists(err) {
					return fmt.Errorf("failed to create IBU during init: %w", err)
				}
			}

			log.Info("Initial IBU created")
			return nil
		}
		return err
	}

	// Strip the ResourceVersion, otherwise the restore fails
	ibu.SetResourceVersion("")

	log.Info("Saved IBU CR found, restoring ...")
	if err := c.Delete(ctx, ibu); err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete IBU during restore: %w", err)
		}
	}

	// Save status as the ibu structure gets over-written by the create call
	// with the result which has no status
	status := ibu.Status
	if err := c.Create(ctx, ibu); err != nil {
		return fmt.Errorf("failed to create IBU to restore: %w", err)
	}

	// Put the saved status into the newly create ibu with the right resource
	// version which is required for the update call to work
	ibu.Status = status
	if err := c.Status().Update(ctx, ibu); err != nil {
		return fmt.Errorf("failed to update IBU during restore: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove IBU in %s: %w", filePath, err)
	}
	log.Info("Restore successful and saved IBU CR removed")
	return nil
}

func ConvertToRawExtension(config any) (runtime.RawExtension, error) {
	rawIgnConfig, err := json.Marshal(config)
	if err != nil {
		return runtime.RawExtension{}, fmt.Errorf("failed to marshal Ignition config: %w", err)
	}

	return runtime.RawExtension{
		Raw: rawIgnConfig,
	}, nil
}

func UpdatePullSecretFromDockerConfig(ctx context.Context, c client.Client, dockerConfigJSON []byte) (*corev1.Secret, error) {
	newPullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.PullSecretName,
			Namespace: common.OpenshiftConfigNamespace,
		},
		Data: map[string][]byte{
			".dockerconfigjson": dockerConfigJSON,
		},
		Type: corev1.SecretTypeDockerConfigJson,
	}

	if err := c.Update(ctx, newPullSecret); err != nil {
		return nil, fmt.Errorf("failed to update pull-secret resource: %w", err)
	}

	return newPullSecret, nil
}

func AppendToListIfNotExists(list []string, value string) []string {
	if lo.Contains(list, value) {
		return list
	}
	return append(list, value)
}

func CreateDynamicClient(kubeconfig string, isTestEnvAllowed bool, log logr.Logger) (dynamic.Interface, error) {
	// Read kubeconfig
	var config *rest.Config
	if _, err := os.Stat(kubeconfig); err != nil {
		if isTestEnvAllowed {
			log.Error(err, "could not find KubeconfigFile. Using empty config only for test environment")
			config = &rest.Config{}
		} else {
			return nil, fmt.Errorf("could not find kubeconfigFile: %w", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("unable to read kubeconfig: %w", err)
		}
	}
	config.Wrap(RetryMiddleware(log)) // allow all client calls to be retriable

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	log.Info("Successfully created dynamic client")
	return dynamicClient, nil
}

func LoadGroupedManifestsFromPath(basePath string, log *logr.Logger) ([][]*unstructured.Unstructured, error) {
	var sortedManifests [][]*unstructured.Unstructured

	groupSubDirs, err := os.ReadDir(filepath.Clean(basePath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read manifest groups subdirs in %s: %w", basePath, err)
	}

	for _, groupSubDir := range groupSubDirs {
		if !groupSubDir.IsDir() {
			log.Info("Unexpected file found, skipping...", "file",
				filepath.Join(basePath, groupSubDir.Name()))
			continue
		}

		// The returned list of entries are sorted by name alphabetically
		manifestDirPath := filepath.Join(basePath, groupSubDir.Name())
		manifestYamls, err := os.ReadDir(filepath.Clean(manifestDirPath))
		if err != nil {
			return nil, fmt.Errorf("failed get manifest yamls in %s: %w", manifestYamls, err)
		}

		var manifests []*unstructured.Unstructured
		for _, yamlFile := range manifestYamls {
			if yamlFile.IsDir() {
				log.Info("Unexpected directory found, skipping...", "directory",
					filepath.Join(manifestDirPath, yamlFile.Name()))
				continue
			}
			yamlFilePath := filepath.Join(manifestDirPath, yamlFile.Name())

			manifest := &unstructured.Unstructured{}
			err := ReadYamlOrJSONFile(yamlFilePath, manifest)
			if err != nil {
				return nil, fmt.Errorf("failed to read manifest file in %s: %w", yamlFilePath, err)
			}
			manifests = append(manifests, manifest)
		}

		sortedManifests = append(sortedManifests, manifests)
	}

	return sortedManifests, nil
}
