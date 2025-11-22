package files

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"
)

// DownloadFile downloads file from the provided url locally;
// if fileName is provided, it will be saved under provided name.
func DownloadFile(fileURL, fileName, targetFolder string) error {
	if fileURL == "" {
		klog.V(100).Info("The fileURL is empty")

		return fmt.Errorf("the fileURL should be provided")
	}

	if fileName == "" {
		klog.V(100).Info("Build fileName from the fullPath")

		fileURL, err := url.Parse(fileURL)
		if err != nil {
			return fmt.Errorf("failed to parse URL %s due to: %w", fileURL, err)
		}

		path := fileURL.Path
		segments := strings.Split(path, "/")
		fileName = segments[len(segments)-1]
	}

	// Get the data
	resp, err := http.Get(fileURL)
	if err != nil {
		return fmt.Errorf("failed to get data from the URL %s due to: %w", fileURL, err)
	}
	defer resp.Body.Close()

	// Create the file
	filePath := filepath.Join(targetFolder, fileName)

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s at folder %s due to: %w", fileName, targetFolder, err)
	}

	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)

	return err
}

// CopyFile copies a file from the provided source path to the provided destination path.
func CopyFile(sourcePath, destinationPath string) error {
	if sourcePath == "" {
		klog.V(100).Info("The source path is empty")

		return fmt.Errorf("the source path should be provided")
	}

	if destinationPath == "" {
		klog.V(100).Info("The destination path is empty")

		return fmt.Errorf("the destination path should be provided")
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer sourceFile.Close()

	// Create the destination file
	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("error creating destination file: %w", err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	err = destinationFile.Sync()
	if err != nil {
		return fmt.Errorf("error syncing destination file: %w", err)
	}

	return err
}
