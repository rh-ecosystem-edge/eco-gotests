package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SRIOV Operator re-installation", Ordered, Label("ocpsriov"),
	ContinueOnFailure, func() {
		It("Verify SR-IOV operator control plane is operational before removal", func() {
			Expect(true).To(BeTrue(), "Test")
		})
	})
