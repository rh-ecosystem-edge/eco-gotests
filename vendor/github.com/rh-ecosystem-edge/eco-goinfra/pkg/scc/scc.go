package scc

import (
	"context"
	"fmt"

	"github.com/golang/glog"
	securityV1 "github.com/openshift/api/security/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const redefiningMsg = "Redefining SecurityContextConstraints"

// Builder provides struct for SecurityContextConstraints object containing connection
// to the cluster SecurityContextConstraints definition.
type Builder struct {
	// SecurityContextConstraints definition. Used to create SecurityContextConstraints object
	Definition *securityV1.SecurityContextConstraints
	// Created SecurityContextConstraints object
	Object *securityV1.SecurityContextConstraints
	// Used in functions that define or mutate SecurityContextConstraints definition. errorMsg is processed
	// before the SecurityContextConstraints object is created
	errorMsg  string
	apiClient *clients.Settings
}

// SecurityContextConstraintsAdditionalOptions additional options for SecurityContextConstraints object.
type SecurityContextConstraintsAdditionalOptions func(builder *Builder) (*Builder, error)

// NewBuilder creates new instance of Builder.
func NewBuilder(apiClient *clients.Settings, name, runAsUser, selinuxContext string) *Builder {
	glog.V(100).Infof(
		"Initializing new SecurityContextConstraints structure with the following params: "+
			"name: %s, runAsUser type: %s, selinuxContext type: %s", name, runAsUser, selinuxContext)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(securityV1.Install)
	if err != nil {
		glog.V(100).Infof("Failed to add security v1 scheme to client schemes")

		return nil
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &securityV1.SecurityContextConstraints{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			RunAsUser: securityV1.RunAsUserStrategyOptions{
				Type: securityV1.RunAsUserStrategyType(runAsUser),
			},
			SELinuxContext: securityV1.SELinuxContextStrategyOptions{
				Type: securityV1.SELinuxContextStrategyType(selinuxContext),
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the SecurityContextConstraints is empty")

		builder.errorMsg = "securityContextConstraints 'name' cannot be empty"

		return builder
	}

	if runAsUser == "" {
		glog.V(100).Infof("The runAsUser of the SecurityContextConstraints is empty")

		builder.errorMsg = "securityContextConstraints 'runAsUser' cannot be empty"

		return builder
	}

	if selinuxContext == "" {
		glog.V(100).Infof("The selinuxContext of the SecurityContextConstraints is empty")

		builder.errorMsg = "securityContextConstraints 'selinuxContext' cannot be empty"

		return builder
	}

	return builder
}

// Pull pulls existing SecurityContextConstraints from cluster.
func Pull(apiClient *clients.Settings, name string) (*Builder, error) {
	glog.V(100).Infof("Pulling existing SecurityContextConstraints object name %s from cluster", name)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil, fmt.Errorf("the apiClient cannot be nil")
	}

	err := apiClient.AttachScheme(securityV1.Install)
	if err != nil {
		glog.V(100).Infof("Failed to add security v1 scheme to client schemes")

		return nil, fmt.Errorf("failed to add security v1 scheme to client schemes")
	}

	builder := &Builder{
		apiClient: apiClient,
		Definition: &securityV1.SecurityContextConstraints{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the SecurityContextConstraints is empty")

		return nil, fmt.Errorf("securityContextConstraints 'name' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("securityContextConstraints object %s does not exist", name)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// WithPrivilegedContainer adds bool flag to the allowPrivilegedContainer of SecurityContextConstraints.
func (builder *Builder) WithPrivilegedContainer(allowPrivileged bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with AllowPrivilegedContainer: %t flag",
		redefiningMsg, builder.Definition.Name, allowPrivileged)

	builder.Definition.AllowPrivilegedContainer = allowPrivileged

	return builder
}

// WithPrivilegedEscalation adds bool flag to the allowPrivilegeEscalation of SecurityContextConstraints.
func (builder *Builder) WithPrivilegedEscalation(allowPrivilegedEscalation bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowPrivilegedEscalation: %t flag",
		redefiningMsg, builder.Definition.Name, allowPrivilegedEscalation)

	builder.Definition.DefaultAllowPrivilegeEscalation = &allowPrivilegedEscalation

	return builder
}

// WithHostDirVolumePlugin adds bool flag to the allowHostDirVolumePlugin of SecurityContextConstraints.
func (builder *Builder) WithHostDirVolumePlugin(allowPlugin bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowHostDirVolumePlugin: %t flag",
		redefiningMsg, builder.Definition.Name, allowPlugin)

	builder.Definition.AllowHostDirVolumePlugin = allowPlugin

	return builder
}

// WithHostIPC adds bool flag to the allowHostIPC of SecurityContextConstraints.
func (builder *Builder) WithHostIPC(allowHostIPC bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowHostIPC: %t flag",
		redefiningMsg, builder.Definition.Name, allowHostIPC)

	builder.Definition.AllowHostIPC = allowHostIPC

	return builder
}

// WithHostNetwork adds bool flag to the allowHostNetwork of SecurityContextConstraints.
func (builder *Builder) WithHostNetwork(allowHostNetwork bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowHostNetwork: %t flag",
		redefiningMsg, builder.Definition.Name, allowHostNetwork)

	builder.Definition.AllowHostNetwork = allowHostNetwork

	return builder
}

// WithHostPID adds bool flag to the allowHostPID of SecurityContextConstraints.
func (builder *Builder) WithHostPID(allowHostPID bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowHostPID: %t flag", redefiningMsg, builder.Definition.Name, allowHostPID)

	builder.Definition.AllowHostPID = allowHostPID

	return builder
}

// WithHostPorts adds bool flag to the allowHostPorts of SecurityContextConstraints.
func (builder *Builder) WithHostPorts(allowHostPorts bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowHostPorts: %t flag",
		redefiningMsg, builder.Definition.Name, allowHostPorts)

	builder.Definition.AllowHostPorts = allowHostPorts

	return builder
}

// WithReadOnlyRootFilesystem adds bool flag to the readOnlyRootFilesystem of SecurityContextConstraints.
func (builder *Builder) WithReadOnlyRootFilesystem(readOnlyRootFilesystem bool) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with readOnlyRootFilesystem: %t flag",
		redefiningMsg, builder.Definition.Name, readOnlyRootFilesystem)

	builder.Definition.ReadOnlyRootFilesystem = readOnlyRootFilesystem

	return builder
}

// WithDropCapabilities adds list of drop capabilities to SecurityContextConstraints.
func (builder *Builder) WithDropCapabilities(requiredDropCapabilities []corev1.Capability) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with requiredDropCapabilities: %v",
		redefiningMsg, builder.Definition.Name, requiredDropCapabilities)

	if len(requiredDropCapabilities) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'requiredDropCapabilities' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'requiredDropCapabilities' cannot be empty list"

		return builder
	}

	if builder.Definition.RequiredDropCapabilities == nil {
		builder.Definition.RequiredDropCapabilities = requiredDropCapabilities

		return builder
	}

	builder.Definition.RequiredDropCapabilities = append(
		builder.Definition.RequiredDropCapabilities, requiredDropCapabilities...)

	return builder
}

// WithAllowCapabilities adds list of allow capabilities to SecurityContextConstraints.
func (builder *Builder) WithAllowCapabilities(allowCapabilities []corev1.Capability) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with allowCapabilities: %v",
		redefiningMsg, builder.Definition.Name, allowCapabilities)

	if len(allowCapabilities) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'allowCapabilities' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'allowCapabilities' cannot be empty list"

		return builder
	}

	if builder.Definition.AllowedCapabilities == nil {
		builder.Definition.AllowedCapabilities = allowCapabilities

		return builder
	}

	builder.Definition.AllowedCapabilities = append(builder.Definition.AllowedCapabilities, allowCapabilities...)

	return builder
}

// WithDefaultAddCapabilities adds list of defaultAddCapabilities to SecurityContextConstraints.
func (builder *Builder) WithDefaultAddCapabilities(defaultAddCapabilities []corev1.Capability) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with defaultAddCapabilities: %v",
		redefiningMsg, builder.Definition.Name, defaultAddCapabilities)

	if len(defaultAddCapabilities) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'defaultAddCapabilities' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'defaultAddCapabilities' cannot be empty list"

		return builder
	}

	if builder.Definition.DefaultAddCapabilities == nil {
		builder.Definition.DefaultAddCapabilities = defaultAddCapabilities

		return builder
	}

	builder.Definition.DefaultAddCapabilities = append(builder.Definition.DefaultAddCapabilities,
		defaultAddCapabilities...)

	return builder
}

// WithPriority adds priority to SecurityContextConstraints.
func (builder *Builder) WithPriority(priority *int32) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with priority: %v", redefiningMsg, builder.Definition.Name, priority)

	builder.Definition.Priority = priority

	return builder
}

// WithFSGroup adds fsGroup to SecurityContextConstraints.
func (builder *Builder) WithFSGroup(fsGroup string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with fsGroup: %s", redefiningMsg, builder.Definition.Name, fsGroup)

	if fsGroup == "" {
		glog.V(100).Infof("SecurityContextConstraints 'fsGroup' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'fsGroup' cannot be empty string"

		return builder
	}

	builder.Definition.FSGroup.Type = securityV1.FSGroupStrategyType(fsGroup)

	return builder
}

// WithFSGroupRange adds fsGroupRange to SecurityContextConstraints.
func (builder *Builder) WithFSGroupRange(fsGroupMin, fsGroupMax int64) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with fsGroupRange: fsGroupMin: %d, fsGroupMax: %d",
		redefiningMsg, builder.Definition.Name, fsGroupMin, fsGroupMax)

	if fsGroupMin > fsGroupMax {
		glog.V(100).Infof("SecurityContextConstraints 'fsGroupMin' argument can not be greater than fsGroupMax")

		builder.errorMsg = "securityContextConstraints 'fsGroupMin' argument can not be greater than fsGroupMax"

		return builder
	}

	if builder.Definition.FSGroup.Ranges == nil {
		builder.Definition.FSGroup.Ranges = []securityV1.IDRange{{Min: fsGroupMin, Max: fsGroupMax}}

		return builder
	}

	builder.Definition.FSGroup.Ranges = append(
		builder.Definition.FSGroup.Ranges, securityV1.IDRange{Max: fsGroupMax, Min: fsGroupMin})

	return builder
}

// WithGroups adds groups to SecurityContextConstraints.
func (builder *Builder) WithGroups(groups []string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with groups: %v", redefiningMsg, builder.Definition.Name, groups)

	if len(groups) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'groups' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'fsGroupType' cannot be empty string"

		return builder
	}

	if builder.Definition.Groups == nil {
		builder.Definition.Groups = groups

		return builder
	}

	builder.Definition.Groups = append(builder.Definition.Groups, groups...)

	return builder
}

// WithSeccompProfiles adds list of seccompProfiles to SecurityContextConstraints.
func (builder *Builder) WithSeccompProfiles(seccompProfiles []string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with SeccompProfiles: %v",
		redefiningMsg, builder.Definition.Name, seccompProfiles)

	if len(seccompProfiles) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'seccompProfiles' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'seccompProfiles' cannot be empty list"

		return builder
	}

	if builder.Definition.SeccompProfiles == nil {
		builder.Definition.SeccompProfiles = seccompProfiles

		return builder
	}

	builder.Definition.SeccompProfiles = append(builder.Definition.SeccompProfiles, seccompProfiles...)

	return builder
}

// WithSupplementalGroups adds SupplementalGroups to SecurityContextConstraints.
func (builder *Builder) WithSupplementalGroups(supplementalGroupsType string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with supplementalGroupsType: %s",
		redefiningMsg, builder.Definition.Name, supplementalGroupsType)

	if supplementalGroupsType == "" {
		glog.V(100).Infof("SecurityContextConstraints 'SupplementalGroups' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'SupplementalGroups' cannot be empty string"

		return builder
	}

	builder.Definition.SupplementalGroups.Type = securityV1.SupplementalGroupsStrategyType(supplementalGroupsType)

	return builder
}

// WithUsers adds users to SecurityContextConstraints.
func (builder *Builder) WithUsers(users []string) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with users: %v", redefiningMsg, builder.Definition.Name, users)

	if len(users) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'users' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'users' cannot be empty list"

		return builder
	}

	if builder.Definition.Users == nil {
		builder.Definition.Users = users

		return builder
	}

	builder.Definition.Users = append(builder.Definition.Users, users...)

	return builder
}

// WithVolumes adds list of volumes to SecurityContextConstraints.
func (builder *Builder) WithVolumes(volumes []securityV1.FSType) *Builder {
	if valid, _ := builder.validate(); !valid {
		return builder
	}

	glog.V(100).Infof("%s %s with volumes: %v", redefiningMsg, builder.Definition.Name, volumes)

	if len(volumes) == 0 {
		glog.V(100).Infof("SecurityContextConstraints 'volumes' argument cannot be empty")

		builder.errorMsg = "securityContextConstraints 'volumes' cannot be empty list"

		return builder
	}

	if builder.Definition.Volumes == nil {
		builder.Definition.Volumes = volumes

		return builder
	}

	builder.Definition.Volumes = append(builder.Definition.Volumes, volumes...)

	return builder
}

// Create generates a SecurityContextConstraints and stores the created object in struct.
func (builder *Builder) Create() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating SecurityContextConstraints %s", builder.Definition.Name)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err != nil {
			glog.V(100).Infof("Failed to create SecurityContextConstraints")

			return nil, err
		}
	}

	builder.Object = builder.Definition

	return builder, err
}

// Delete removes a SecurityContextConstraints.
func (builder *Builder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Removing SecurityContextConstraints %s", builder.Definition.Name)

	if !builder.Exists() {
		glog.V(100).Infof("SecurityContextConstraints %s does not exist", builder.Definition.Name)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return err
	}

	builder.Object = nil

	return nil
}

// Update modifies an existing SecurityContextConstraints in the cluster.
func (builder *Builder) Update() (*Builder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Updating SecurityContextConstraints %s ", builder.Definition.Name)

	if !builder.Exists() {
		return nil, fmt.Errorf("failed to update SecurityContextConstraints, object does not exist on cluster")
	}

	err := builder.apiClient.Update(context.TODO(), builder.Definition)

	return builder, err
}

// Get returns NMState object if found.
func (builder *Builder) Get() (*securityV1.SecurityContextConstraints, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Collecting SecurityContextConstraints object %s", builder.Definition.Name)

	scc := &securityV1.SecurityContextConstraints{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{Name: builder.Definition.Name}, scc)
	if err != nil {
		glog.V(100).Infof("SecurityContextConstraints object %s does not exist", builder.Definition.Name)

		return nil, err
	}

	return scc, nil
}

// Exists checks whether the given SecurityContextConstraints exists.
func (builder *Builder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if SecurityContextConstraints %s exists", builder.Definition.Name)

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		glog.V(100).Infof("Failed to collect SecurityContextConstraints object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *Builder) validate() (bool, error) {
	resourceCRD := "SecurityContextConstraints"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
