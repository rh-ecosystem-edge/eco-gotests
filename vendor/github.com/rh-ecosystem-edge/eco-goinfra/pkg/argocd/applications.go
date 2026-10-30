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
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// ApplicationBuilder provides a struct for an application object from the cluster and a definition.
type ApplicationBuilder struct {
	common.EmbeddableBuilder[argocdtypes.Application, *argocdtypes.Application]
	common.EmbeddableCreator[argocdtypes.Application, ApplicationBuilder, *argocdtypes.Application, *ApplicationBuilder]
	common.EmbeddableDeleteReturner[argocdtypes.Application, ApplicationBuilder, *argocdtypes.Application, *ApplicationBuilder]
	common.EmbeddableForceUpdater[argocdtypes.Application, ApplicationBuilder, *argocdtypes.Application, *ApplicationBuilder]
}

// AttachMixins wires the embedded CRUD mixins to this builder instance.
func (builder *ApplicationBuilder) AttachMixins() {
	builder.EmbeddableCreator.SetBase(builder)
	builder.EmbeddableDeleteReturner.SetBase(builder)
	builder.EmbeddableForceUpdater.SetBase(builder)
}

// GetGVK returns the Application GVK for this builder.
func (builder *ApplicationBuilder) GetGVK() schema.GroupVersionKind {
	return argocdtypes.ApplicationSchemaGroupVersionKind
}

// NewApplicationBuilder creates a new ApplicationBuilder instance.
func NewApplicationBuilder(apiClient *clients.Settings, name, nsname string) *ApplicationBuilder {
	return common.NewNamespacedBuilder[argocdtypes.Application, ApplicationBuilder](
		apiClient, argocdtypes.AddToScheme, name, nsname)
}

// PullApplication pulls existing application into ApplicationBuilder struct.
func PullApplication(apiClient *clients.Settings, name, nsname string) (*ApplicationBuilder, error) {
	return common.PullNamespacedBuilder[argocdtypes.Application, ApplicationBuilder](
		context.TODO(), apiClient, argocdtypes.AddToScheme, name, nsname)
}

// WithGitDetails applies git details to application definition.
func (builder *ApplicationBuilder) WithGitDetails(gitRepo, gitBranch, gitPath string) *ApplicationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	if gitRepo == "" {
		klog.V(100).Info("The 'gitRepo' of the argocd application is empty")

		builder.SetError(fmt.Errorf("'gitRepo' parameter is empty"))

		return builder
	}

	if gitBranch == "" {
		klog.V(100).Info("The 'gitBranch' of the argocd application is empty")

		builder.SetError(fmt.Errorf("'gitBranch' parameter is empty"))

		return builder
	}

	if gitPath == "" {
		klog.V(100).Info("The 'gitPath' of the argocd application is empty")

		builder.SetError(fmt.Errorf("'gitPath' parameter is empty"))

		return builder
	}

	klog.V(100).Infof(
		"Adding the following git details to the argocd application: %s in namespace: %s "+
			"RepoURL: %s,TargetRevision: %s, Path: %s", builder.Definition.Name, builder.Definition.Namespace,
		gitRepo, gitBranch, gitPath,
	)

	if builder.Definition.Spec.Source == nil {
		builder.Definition.Spec.Source = &argocdtypes.ApplicationSource{}
	}

	builder.Definition.Spec.Source.RepoURL = gitRepo
	builder.Definition.Spec.Source.TargetRevision = gitBranch
	builder.Definition.Spec.Source.Path = gitPath

	return builder
}

// WithGitPathAppended appends the given elements to the git path of the application source. It is similar to
// [WithGitDetails] but does not change the RepoURL or TargetRevision and only appends the elements to the Path field,
// rather than replaces it.
func (builder *ApplicationBuilder) WithGitPathAppended(elements ...string) *ApplicationBuilder {
	if err := common.Validate(builder); err != nil {
		return builder
	}

	if builder.Definition.Spec.Source == nil {
		klog.V(100).Info("The source of the argocd application is nil")

		builder.SetError(fmt.Errorf("cannot append to git path because the source is nil"))

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
	if err := common.Validate(builder); err != nil {
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
	if err := common.Validate(builder); err != nil {
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
	if err := common.Validate(builder); err != nil {
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
