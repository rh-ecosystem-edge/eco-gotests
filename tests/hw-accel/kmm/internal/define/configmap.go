package define

import (
	"html/template"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
)

// MultiStageConfigMapContent returns the configmap multi-stage contents for a specified module name.
func MultiStageConfigMapContent(module string) map[string]string {
	data := map[string]interface{}{
		"Module": module,
	}

	templateInstance := template.Must(template.New("contents").Parse(kmmparams.MultistageContents))
	builder := &strings.Builder{}

	if err := templateInstance.Execute(builder, data); err != nil {
		panic(err)
	}

	content := builder.String()

	configmapContents := map[string]string{"dockerfile": content}

	return configmapContents
}

// SimpleKmodConfigMapContents returns the configmap for simple-kmod example.
func SimpleKmodConfigMapContents() map[string]string {
	configmapContents := map[string]string{"dockerfile": kmmparams.SimpleKmodContents}

	return configmapContents
}

// SimpleKmodFirmwareConfigMapContents returns the configmap for simple-kmod-firmware example.
func SimpleKmodFirmwareConfigMapContents() map[string]string {
	configmapContents := map[string]string{"dockerfile": kmmparams.SimpleKmodFirmwareContents}

	return configmapContents
}

// LocalMultiStageConfigMapContent returns the configmap multi-stage contents for a specified module name.
func LocalMultiStageConfigMapContent(module string) map[string]string {
	data := map[string]interface{}{
		"Module": module,
	}

	templateInstance := template.Must(template.New("contents").Parse(kmmparams.LocalMultiStageContents))
	builder := &strings.Builder{}

	if err := templateInstance.Execute(builder, data); err != nil {
		panic(err)
	}

	content := builder.String()

	configmapContents := map[string]string{"dockerfile": content}

	return configmapContents
}

// KmmScannerConfigMapContents returns the configmap for KMM image scanner.
func KmmScannerConfigMapContents() map[string]string {
	configMapContent := map[string]string{"dockerfile": kmmparams.KmmScannerDockerfile}

	return configMapContent
}
