package argocd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/logging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplicationBuilder provides a struct for an application object from the cluster and a definition.
type ApplicationBuilder struct {
	// application Definition, used to create the application object.
	Definition *argocdtypes.Application
	// created application object.
	Object *argocdtypes.Application
	// api client to interact with the cluster.
	apiClient runtimeclient.Client
	// used to store latest error message upon defining or mutating application definition.
	errorMsg string
}

// PullApplication pulls existing application into ApplicationBuilder struct.
func PullApplication(apiClient *clients.Settings, name, nsname string) (*ApplicationBuilder, error) {
	klog.V(100).Infof("Pulling existing Application name %s under namespace %s from cluster", name, nsname)

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("application 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(argocdtypes.AddToScheme)
	if err != nil {
		klog.V(100).Info("Failed to add argocd Application scheme to client schemes")

		return nil, err
	}

	builder := &ApplicationBuilder{
		apiClient: apiClient.Client,
		Definition: &argocdtypes.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		klog.V(100).Info("The name of the Application is empty")

		return nil, fmt.Errorf("application 'name' cannot be empty")
	}

	if nsname == "" {
		klog.V(100).Info("The namespace of the Application is empty")

		return nil, fmt.Errorf("application 'namespace' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("application object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Exists checks whether the given argocd application exists.
func (builder *ApplicationBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	klog.V(100).Infof("Checking if argocd app %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns argocd application object if found.
func (builder *ApplicationBuilder) Get() (*argocdtypes.Application, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof("Getting argocd app %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	application := &argocdtypes.Application{}

	err := builder.apiClient.Get(logging.DiscardContext(), runtimeclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, application)
	if err != nil {
		klog.V(100).Infof(
			"Failed to Get Application %s in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, err
	}

	return application, nil
}

// Update renovates the existing argocd application object with the argocd application definition in builder.
func (builder *ApplicationBuilder) Update(force bool) (*ApplicationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Updating the argocd application object %s in namespace %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof(
			"Application %s does not exist in namespace %s", builder.Definition.Name, builder.Definition.Namespace)

		return nil, fmt.Errorf("cannot update non-existent Application")
	}

	builder.Definition.ResourceVersion = builder.Object.ResourceVersion

	err := builder.apiClient.Update(logging.DiscardContext(), builder.Definition)
	if err != nil {
		if force {
			klog.V(100).Infof("%v", msg.FailToUpdateNotification("Application", builder.Definition.Name, builder.Definition.Namespace))

			builder, err := builder.Delete()
			builder.Definition.ResourceVersion = ""

			if err != nil {
				klog.V(100).Infof("%v", msg.FailToUpdateError("Application", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}

		return nil, err
	}

	builder.Object = builder.Definition

	return builder, nil
}

// Delete removes the argocd application object from a cluster.
func (builder *ApplicationBuilder) Delete() (*ApplicationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Deleting the argocd application object %s from namespace: %s", builder.Definition.Name,
		builder.Definition.Namespace)

	if !builder.Exists() {
		klog.V(100).Infof("application %s in namespace %s cannot be deleted because it does not exist",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return builder, nil
	}

	err := builder.apiClient.Delete(logging.DiscardContext(), builder.Object)
	if err != nil {
		return builder, fmt.Errorf("can not delete argocd application: %w", err)
	}

	builder.Object = nil

	return builder, nil
}

// Create makes an argocd application in the cluster and stores the created object in a struct.
func (builder *ApplicationBuilder) Create() (*ApplicationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	klog.V(100).Infof("Creating argocd application %s in namespace: %s", builder.Definition.Name,
		builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(logging.DiscardContext(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// WithGitDetails applies git details to application definition.
func (builder *ApplicationBuilder) WithGitDetails(gitRepo, gitBranch, gitPath string) *ApplicationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if gitRepo == "" {
		klog.V(100).Info("The 'gitRepo' of the argocd application is empty")

		builder.errorMsg = "'gitRepo' parameter is empty"

		return builder
	}

	if gitBranch == "" {
		klog.V(100).Info("The 'gitBranch' of the argocd application is empty")

		builder.errorMsg = "'gitBranch' parameter is empty"

		return builder
	}

	if gitPath == "" {
		klog.V(100).Info("The 'gitPath' of the argocd application is empty")

		builder.errorMsg = "'gitPath' parameter is empty"

		return builder
	}

	klog.V(100).Infof(
		"Adding the following git details to the argocd application: %s in namespace: %s "+
			"RepoURL: %s,TargetRevision: %s, Path: %s", builder.Definition.Name, builder.Definition.Namespace,
		gitRepo, gitBranch, gitPath,
	)

	builder.Definition.Spec.Source.RepoURL = gitRepo
	builder.Definition.Spec.Source.TargetRevision = gitBranch
	builder.Definition.Spec.Source.Path = gitPath

	return builder
}

// WithGitPathAppended appends the given elements to the git path of the application source. It is similar to
// [WithGitDetails] but does not change the RepoURL or TargetRevision and only appends the elements to the Path field,
// rather than replaces it.
func (builder *ApplicationBuilder) WithGitPathAppended(elements ...string) *ApplicationBuilder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	if builder.Definition.Spec.Source == nil {
		klog.V(100).Info("The source of the argocd application is nil")

		builder.errorMsg = "cannot append to git path because the source is nil"

		return builder
	}

	builder.Definition.Spec.Source.Path = path.Join(builder.Definition.Spec.Source.Path, path.Join(elements...))

	return builder
}

// WaitForCondition waits until the Application has a condition that matches the expected, checking only the Type and
// Message fields. For the messages field, it matches if the message contains the expected. Zero value fields in the
// expected condition are ignored.
func (builder *ApplicationBuilder) WaitForCondition(
	expected argocdtypes.ApplicationCondition, timeout time.Duration) (*ApplicationBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	klog.V(100).Infof(
		"Waiting until condition of Argo CD Application %s in namespace %s matches %v",
		builder.Definition.Name, builder.Definition.Namespace, expected)

	if !builder.Exists() {
		return nil, fmt.Errorf(
			"application object %s in namespace %s does not exist", builder.Definition.Name, builder.Definition.Namespace)
	}

	var err error

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof(
					"Failed to get Argo CD Application %s in namespace %s: %s",
					builder.Definition.Name, builder.Definition.Namespace, err.Error())

				return false, nil
			}

			for _, condition := range builder.Object.Status.Conditions {
				if expected.Type != "" && condition.Type != expected.Type {
					continue
				}

				if expected.Message != "" && !strings.Contains(condition.Message, expected.Message) {
					continue
				}

				return true, nil
			}

			return false, nil
		})
	if err != nil {
		return nil, err
	}

	return builder, nil
}

// DoesGitPathExist checks if a path exists in the application's git repository. It does this by sending a HEAD request
// to the URL of the form `<repo-url>/raw/<target-revision>/<path>/<elements>`. If the final element does not end with
// `kustomization.yaml`, it will be appended to the URL.
//
// An expected use of this function may be checking `appBuilder.DoesGitPathExist("ztp-test", "ztp-test-case")` to know
// if the application source can have the path `ztp-test/ztp-test-case` appended.
func (builder *ApplicationBuilder) DoesGitPathExist(elements ...string) bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	if builder.Definition.Spec.Source == nil {
		klog.V(100).Info("The source of the argocd application is nil")

		return false
	}

	repoURL := strings.TrimSuffix(builder.Definition.Spec.Source.RepoURL, ".git")

	rawURL, err := url.ParseRequestURI(repoURL)
	if err != nil {
		klog.V(100).Infof("Failed to parse repo URL %s: %v", builder.Definition.Spec.Source.RepoURL, err)

		return false
	}

	// For GOGS, GitLab, and GitHub, the existence of a file can be checked by sending a HEAD request to the URL of
	// the form `<repo-url>/raw/<target-revision>/<path>`. GitHub will send a redirect but this is followed
	// automatically by the client. For GOGS and GitLab, the HEAD request will return a 200 OK if the file exists.
	rawURL = rawURL.JoinPath("raw", builder.Definition.Spec.Source.TargetRevision, builder.Definition.Spec.Source.Path)
	rawURL = rawURL.JoinPath(elements...)

	// If a directory is provided, the HEAD request will fail so we need to append the kustomization.yaml file. Such
	// a file should exist in the git path directory of the application.
	if !strings.HasSuffix(rawURL.Path, "kustomization.yaml") {
		rawURL = rawURL.JoinPath("kustomization.yaml")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	response, err := client.Head(rawURL.String())
	if err != nil {
		klog.V(100).Infof("Failed to get git path %s: %s", rawURL.String(), err.Error())

		return false
	}

	// Since we do not reuse the client there is no need to read and close the body, but it will not hurt to do so.
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		klog.V(100).Infof("Failed to read response body for git path %s: %s", rawURL.String(), err.Error())

		return false
	}

	// Any redirects should be followed automatically by the client, so anything other than 2xx is an error.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		klog.V(100).Infof("Git path %s does not exist: %s with body %s", rawURL.String(), response.Status, string(body))

		return false
	}

	return true
}

// WaitForSourceUpdate waits up to timeout until the Application has a source that matches the expected, checking only
// the RepoURL, Path, and TargetRevision fields. If synced is true, it will also wait until the Application is synced.
func (builder *ApplicationBuilder) WaitForSourceUpdate(synced bool, timeout time.Duration) error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	klog.V(100).Infof(
		"Waiting until source of Argo CD Application %s in namespace %s is updated with synced=%t",
		builder.Definition.Name, builder.Definition.Namespace, synced)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder.Object, err = builder.Get()
			if err != nil {
				klog.V(100).Infof("Failed to get Argo CD Application %s in namespace %s: %v",
					builder.Definition.Name, builder.Definition.Namespace, err)

				return false, nil
			}

			expectedSource := builder.Object.Spec.Source
			if expectedSource == nil {
				klog.V(100).Infof("Application %s in namespace %s has no source",
					builder.Definition.Name, builder.Definition.Namespace)

				return false, nil
			}

			actualSource := builder.Object.Status.Sync.ComparedTo.Source
			if actualSource.RepoURL != expectedSource.RepoURL ||
				actualSource.Path != expectedSource.Path ||
				actualSource.TargetRevision != expectedSource.TargetRevision {
				klog.V(100).Infof("Application %s in namespace %s has source %v, expected %v",
					builder.Definition.Name, builder.Definition.Namespace, actualSource, expectedSource)

				return false, nil
			}

			if synced && builder.Object.Status.Sync.Status != argocdtypes.SyncStatusCodeSynced {
				klog.V(100).Infof("Application %s in namespace %s is not synced, status: %s",
					builder.Definition.Name, builder.Definition.Namespace, builder.Object.Status.Sync.Status)

				return false, nil
			}

			return true, nil
		})
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *ApplicationBuilder) validate() (bool, error) {
	resourceCRD := "Application"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
