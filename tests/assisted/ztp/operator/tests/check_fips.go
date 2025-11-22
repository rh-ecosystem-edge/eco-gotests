package operator_test

import (
	"bytes"
	"debug/elf"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/installconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpparams"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
)

const (
	tmpDir                        = "temp_go"
	assistedServiceBinary         = "assisted-service"
	assistedImageContainer        = "assisted-service"
	assistedImageServiceBinary    = "assisted-image-service"
	assistedImageServiceContainer = "assisted-image-service"
	installConfigConfMap          = "cluster-config-v1"
	installConfigConfMapNS        = "kube-system"
)

var _ = Describe(
	"Openshift HUB cluster and AI are FIPS ready",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelFipsVerificationTestCases), func() {
		var (
			fipsEnabledOnHub bool
		)
		When("on MCE 2.0 and above", func() {

			BeforeAll(func() {
				By("Creating temp directory")
				Expect(createTmpDir(tmpDir)).NotTo(HaveOccurred(), "could not create temp directory "+tmpDir)
				By("Getting configmap")
				Expect(ZTPConfig.HubInstallConfig.Object.Data["install-config"]).ToNot(BeEmpty(),
					"error pulling install-config from HUB cluster")
				installConfigData, err := installconfig.NewInstallConfigFromString(
					ZTPConfig.HubInstallConfig.Object.Data["install-config"])
				Expect(err).NotTo(HaveOccurred(), "error reading in install-config as yaml")
				fipsEnabledOnHub = installConfigData.FIPS
			})

			It("Testing assisted-service is FIPS ready", reportxml.ID("65866"), func() {
				By("Extracting the assisted-service binary")
				Expect(extractBinaryFromPod(
					ZTPConfig.HubAssistedServicePod(),
					assistedServiceBinary,
					assistedImageContainer,
					false, tmpDir)).ToNot(HaveOccurred(), "error extracting binary")

				By("Testing assisted-service was compiled with CGO_ENABLED=1")
				result, err := testBinaryCgoEnabled(tmpDir + "/" + assistedServiceBinary)
				Expect(err).ToNot(HaveOccurred(), "error extracting assisted-service binary")
				Expect(result).To(BeTrue(), "assisted service binary is compiled with CGO_ENABLED=1")
			})
			It("Testing assisted-image-service is FIPS ready", reportxml.ID("65867"), func() {
				By("Extracting the assisted-image-service binary")
				Expect(extractBinaryFromPod(
					ZTPConfig.HubAssistedImageServicePod(),
					assistedImageServiceBinary,
					assistedImageServiceContainer,
					false, tmpDir)).ToNot(
					HaveOccurred(), "error extracting binary")

				By("Testing assisted-image-service was compiled with CGO_ENABLED=1")
				result, err := testBinaryCgoEnabled(tmpDir + "/" + assistedImageServiceBinary)
				Expect(err).ToNot(HaveOccurred(), "error extracting assisted-image-service binary")
				Expect(result).To(BeTrue(), "assisted-image-service binary is not compiled with CGO_ENABLED=1")
			})

			It("Assert HUB cluster was deployed with FIPS", reportxml.ID("65849"), func() {
				if !fipsEnabledOnHub {
					Skip("hub is not installed with fips")
				}
				Expect(fipsEnabledOnHub).To(BeTrue(),
					"hub cluster is not installed with fips")
			})

			AfterAll(func() {
				By("Removing temp directory")
				Expect(removeTmpDir(tmpDir)).NotTo(HaveOccurred(), "could not remove temp directory "+tmpDir)
			})
		})
	})

func createTmpDir(dirPath string) error {
	var err error
	if _, err = os.Stat(dirPath); errors.Is(err, fs.ErrNotExist) {
		return os.Mkdir(dirPath, 0777)
	}

	return err
}

func removeTmpDir(dirPath string) error {
	var err error
	if _, err = os.Stat(dirPath); err == nil {
		return os.RemoveAll(dirPath)
	}

	return err
}

func testBinaryCgoEnabled(path string) (bool, error) {
	file, err := elf.Open(path)
	if err != nil {
		klog.V(ztpparams.ZTPLogLevel).Infof("Got error: %s", err)

		return false, err
	}
	defer file.Close()

	buildInfo := file.Section(".go.buildinfo")
	if buildInfo == nil {
		return false, fmt.Errorf("no BUILDINFO data found")
	}

	content, err := buildInfo.Data()
	if err != nil {
		return false, err
	}

	if strings.Contains(string(content), "CGO_ENABLED=1") {
		klog.V(ztpparams.ZTPLogLevel).Info(string(content))

		return true, nil
	}

	return false, nil
}

func extractBinaryFromPod(
	podbuilder *pod.Builder,
	binary string,
	containerName string,
	tar bool,
	destination string) error {
	if !podbuilder.Exists() {
		klog.V(ztpparams.ZTPLogLevel).Infof("Pod doesn't exist")

		return fmt.Errorf("%s pod does not exist", podbuilder.Definition.Name)
	}

	var buf bytes.Buffer

	buf, err = podbuilder.Copy(binary, containerName, tar)
	if err != nil {
		klog.V(ztpparams.ZTPLogLevel).Infof("Got error reading file to buffer. The error is : %s", err)

		return err
	}

	err = os.WriteFile(destination+"/"+binary, buf.Bytes(), 0666)
	if err != nil {
		klog.V(ztpparams.ZTPLogLevel).
			Infof("Got error writing to %s/%s from buffer. The error is: %s", destination, binary, err)

		return err
	}

	return nil
}
