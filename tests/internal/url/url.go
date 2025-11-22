package url

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cavaliergopher/grab/v3"
	"k8s.io/klog/v2"
)

// Fetch retrieves specified URL using GET or HEAD method.
func Fetch(url, method string, skipCertVerify bool) (string, int, error) {
	var (
		res *http.Response
		err error
	)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipCertVerify},
	}

	client := &http.Client{Transport: tr}

	klog.V(90).Infof("Attempt to retrieve %v with %s method\n", url, strings.ToUpper(method))

	switch {
	case strings.EqualFold(method, "get"):
		res, err = client.Get(url)
	case strings.EqualFold(method, "head"):
		res, err = client.Head(url)
	default:
		klog.Warning(fmt.Sprintf("Unsupported method: %v\n", method))

		return "", 0, fmt.Errorf("unsupported method %v", method)
	}

	if err != nil {
		klog.Warning(fmt.Sprintf("Error accessing %s ; Reason %v\n", url, err))

		return "", 0, fmt.Errorf("error accessing %s ; Reason %w", url, err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		klog.Warning(fmt.Sprintf("Error reading reply: %v\n", err))

		return "", 0, fmt.Errorf("error reading reply: %w", err)
	}

	klog.V(50).Infof("\tReply: %s\n", body)
	klog.V(50).Infof("\tStatus: %s\n", res.Status)

	for key, value := range res.Header {
		klog.V(50).Infof("  %v: %v\n", key, value)
	}

	return string(body), res.StatusCode, nil
}

// DownloadToDir saves content from the specified URL under the specified folder.
func DownloadToDir(url, dirName string, skipCertVerify bool) error {
	grabClient := grab.NewClient()

	if skipCertVerify {
		transport, ok := grabClient.HTTPClient.(*http.Client).Transport.(*http.Transport)
		if !ok {
			return fmt.Errorf("error: received unexpected http client")
		}

		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	klog.V(50).Infof("Attempting to save content from %s into directory %s", url, dirName)

	grabRequest, err := grab.NewRequest(dirName, url)
	if err != nil {
		return err
	}

	grabResponse := grabClient.Do(grabRequest)
	klog.V(50).Infof("HTTP response status: %v", grabResponse.HTTPResponse.Status)

	if err := grabResponse.Err(); err != nil {
		return err
	}

	return nil
}
