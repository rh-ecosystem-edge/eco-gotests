package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"
)

// dummyProblemDetails is a test problem details object for use in tests where the API returns a 500 error.
var dummyProblemDetails = common.ProblemDetails{
	Status: 500,
	Title:  ptr.To("Internal Server Error"),
	Detail: "Internal server error occurred",
}

// jsonResponseHandler returns an http.HandlerFunc that serves a JSON response.
func jsonResponseHandler(response any, statusCode ...int) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		code := http.StatusOK
		if len(statusCode) > 0 {
			code = statusCode[0]
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(code)

		if response != nil {
			_ = json.NewEncoder(writer).Encode(response)
		}
	}
}

// validateHTTPRequest validates the common fields of an http.Request.
func validateHTTPRequest(
	t *testing.T, req *http.Request, method, path string, queryParams map[string]string, contentType ...string) {
	t.Helper()

	assert.Equal(t, method, req.Method)
	assert.Equal(t, path, req.URL.Path)

	for key, expectedValue := range queryParams {
		assert.Equal(t, expectedValue, req.URL.Query().Get(key))
	}

	if len(contentType) > 0 {
		assert.Equal(t, contentType[0], req.Header.Get("Content-Type"))
	}
}
