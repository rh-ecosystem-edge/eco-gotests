package tests

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/dra/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/internal/check"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/neuron/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	draClusterRoleName = "awslabs-gpu-operator-dra-driver"
	draServiceAccount  = "awslabs-gpu-operator-dra-driver"
)

var _ = Describe("Neuron DRA RBAC Tests", Ordered,
	Label(params.Label, params.DRALabel, tsparams.LabelValidation), func() {
		Context("DRA driver RBAC", Label(tsparams.LabelSuite), func() {
			It("should have DRA driver ServiceAccount",
				reportxml.ID("90474"), func() {
					By("Checking ServiceAccount exists")

					serviceAccount, err := APIClient.K8sClient.CoreV1().ServiceAccounts(
						params.NeuronNamespace).Get(
						context.TODO(), draServiceAccount, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"ServiceAccount %s should exist in namespace %s",
						draServiceAccount, params.NeuronNamespace)

					By("Verifying ClusterRoleBinding connects SA to ClusterRole")

					crb, err := APIClient.K8sClient.RbacV1().ClusterRoleBindings().Get(
						context.TODO(), draClusterRoleName, metav1.GetOptions{})
					Expect(err).ToNot(HaveOccurred(),
						"ClusterRoleBinding %s should exist", draClusterRoleName)
					Expect(crb.RoleRef.Name).To(Equal(draClusterRoleName),
						"ClusterRoleBinding should reference ClusterRole %s", draClusterRoleName)

					foundSubject := false

					for _, subject := range crb.Subjects {
						if subject.Kind == "ServiceAccount" &&
							subject.Name == serviceAccount.Name &&
							subject.Namespace == params.NeuronNamespace {
							foundSubject = true

							break
						}
					}

					Expect(foundSubject).To(BeTrue(),
						"ClusterRoleBinding should have SA %s/%s as subject",
						params.NeuronNamespace, draServiceAccount)

					klog.V(params.NeuronLogLevel).Infof(
						"ServiceAccount %s bound to ClusterRole %s", serviceAccount.Name, draClusterRoleName)
				})

			It("should have ResourceSlices CRUD permissions",
				reportxml.ID("90475"), func() {
					By("Getting DRA ClusterRole")

					clusterRole, err := check.GetClusterRole(APIClient, draClusterRoleName)
					Expect(err).ToNot(HaveOccurred(),
						"ClusterRole %s should exist", draClusterRoleName)

					By("Verifying resourceslices CRUD")

					expectedVerbs := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
					Expect(check.HasPolicyRule(clusterRole.Rules, "resource.k8s.io", "resourceslices", expectedVerbs)).To(BeTrue(),
						"ClusterRole should grant %v on resourceslices", expectedVerbs)

					klog.V(params.NeuronLogLevel).Info("ResourceSlices CRUD permissions verified")
				})

			It("should have ResourceClaims read permissions",
				reportxml.ID("90476"), func() {
					clusterRole, err := check.GetClusterRole(APIClient, draClusterRoleName)
					Expect(err).ToNot(HaveOccurred())

					expectedVerbs := []string{"get", "list", "watch"}
					Expect(check.HasPolicyRule(clusterRole.Rules, "resource.k8s.io", "resourceclaims", expectedVerbs)).To(BeTrue(),
						"ClusterRole should grant %v on resourceclaims", expectedVerbs)

					klog.V(params.NeuronLogLevel).Info("ResourceClaims read permissions verified")
				})

			It("should have DeviceClasses read permissions",
				reportxml.ID("90477"), func() {
					clusterRole, err := check.GetClusterRole(APIClient, draClusterRoleName)
					Expect(err).ToNot(HaveOccurred())

					expectedVerbs := []string{"get", "list", "watch"}
					Expect(check.HasPolicyRule(clusterRole.Rules, "resource.k8s.io", "deviceclasses", expectedVerbs)).To(BeTrue(),
						"ClusterRole should grant %v on deviceclasses", expectedVerbs)

					klog.V(params.NeuronLogLevel).Info("DeviceClasses read permissions verified")
				})

			It("should have nodes read and status patch permissions",
				reportxml.ID("90478"), func() {
					clusterRole, err := check.GetClusterRole(APIClient, draClusterRoleName)
					Expect(err).ToNot(HaveOccurred())

					By("Verifying nodes read + patch + update")

					nodeVerbs := []string{"get", "list", "watch", "patch", "update"}
					Expect(check.HasPolicyRule(clusterRole.Rules, "", "nodes", nodeVerbs)).To(BeTrue(),
						"ClusterRole should grant %v on nodes", nodeVerbs)

					By("Verifying nodes/status patch")

					statusVerbs := []string{"patch"}
					Expect(check.HasPolicyRule(clusterRole.Rules, "", "nodes/status", statusVerbs)).To(BeTrue(),
						"ClusterRole should grant %v on nodes/status", statusVerbs)

					klog.V(params.NeuronLogLevel).Info("Nodes read and status patch permissions verified")
				})

			It("should have privileged SCC use permission",
				reportxml.ID("90479"), func() {
					clusterRole, err := check.GetClusterRole(APIClient, draClusterRoleName)
					Expect(err).ToNot(HaveOccurred())

					Expect(check.HasSCCRule(clusterRole.Rules, "privileged")).To(BeTrue(),
						fmt.Sprintf("ClusterRole %s should grant 'use' on privileged SCC",
							draClusterRoleName))

					klog.V(params.NeuronLogLevel).Info("Privileged SCC use permission verified")
				})
		})
	})
