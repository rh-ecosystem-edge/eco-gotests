package define

import (
	"bytes"
	"html/template"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
)

var secretTemplate = template.Must(template.New("contents").Parse(kmmparams.SecretContents))

// SecretContent returns the secret based on the registry and secret.
func SecretContent(registry, pullSecret string) map[string][]byte {
	data := map[string]interface{}{
		"Registry":   registry,
		"PullSecret": pullSecret,
	}

	var buf bytes.Buffer

	if err := secretTemplate.Execute(&buf, data); err != nil {
		panic(err)
	}

	secretContents := map[string][]byte{".dockerconfigjson": buf.Bytes()}

	return secretContents
}
