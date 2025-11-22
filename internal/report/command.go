package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"path"
	"strings"

	"k8s.io/klog/v2"
)

// CloneRepo clones the eco-gotests repo from the given repo and branch and returns the path to the cloned repo.
func CloneRepo(ctx context.Context, localPath, repo, branch string) (string, error) {
	clonedPath := path.Join(localPath, "eco-gotests")

	klog.V(100).Infof("Cloning repo %s with branch %s to %s", repo, branch, clonedPath)

	err := os.RemoveAll(clonedPath)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "-b", branch, repo, "eco-gotests")
	cmd.Dir = localPath

	err = execCommand(cmd)
	if err != nil {
		return "", err
	}

	return clonedPath, nil
}

// DryRun runs the eco-gotests tests in dry-run mode and returns the path to the JSON report file.
func DryRun(ctx context.Context, clonedPath string) (string, error) {
	klog.V(100).Infof("Running eco-gotests dry-run in %s", clonedPath)

	cmd := exec.CommandContext(ctx, "ginkgo", "--json-report=report.json", "-dry-run", "-v", "-r", "./tests")
	cmd.Dir = clonedPath
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "ECO_DRY_RUN=true")

	err := execCommand(cmd)
	if err != nil {
		return "", err
	}

	reportPath := path.Join(clonedPath, "report.json")

	return reportPath, nil
}

// GetRepoRevision returns the current revision of the repo at the given path.
func GetRepoRevision(ctx context.Context, repoPath string) (string, error) {
	klog.V(100).Infof("Getting repo revision for %s", repoPath)

	cmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath

	stdout, err := execCommandWithStdout(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

// GetRepoBranch returns the current branch of the repo at the given path.
func GetRepoBranch(ctx context.Context, repoPath string) (string, error) {
	klog.V(100).Infof("Getting repo branch for %s", repoPath)

	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = repoPath

	stdout, err := execCommandWithStdout(cmd)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

// HasLocalChanges returns true if the repo at the given path has uncommitted changes and false otherwise.
func HasLocalChanges(ctx context.Context, repoPath string) (bool, error) {
	klog.V(100).Infof("Checking for local changes in %s", repoPath)

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = repoPath

	stdout, err := execCommandWithStdout(cmd)
	if err != nil {
		return false, err
	}

	return len(stdout) > 0, nil
}

// GetRemoteRevisions returns a map of branches to revisions executing just one command. Provided branches will not
// appear in the returned map if they are not found on the remote repo.
func GetRemoteRevisions(ctx context.Context, repo string, branches iter.Seq[string]) (map[string]string, error) {
	klog.V(100).Infof("Getting remote revisions for repo %s", repo)

	args := []string{"ls-remote", repo}
	for branch := range branches {
		args = append(args, "refs/heads/"+branch)
	}

	klog.V(100).Infof("Getting remote revisions with arguments %+v", args)
	cmd := exec.CommandContext(ctx, "git", args...)

	stdout, err := execCommandWithStdout(cmd)
	if err != nil {
		return nil, err
	}

	revisions := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(stdout))

	for scanner.Scan() {
		revision, branch, found := strings.Cut(scanner.Text(), "\t")
		if !found {
			return nil, fmt.Errorf("failed to parse remote revision from `%s`", scanner.Text())
		}

		branch = strings.TrimPrefix(branch, "refs/heads/")
		revisions[branch] = revision
	}

	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return revisions, nil
}

// execCommand executes the given command and returns an error if it fails. It captures the stdout and stderr of the
// command and logs them if the command fails.
func execCommand(command *exec.Cmd) error {
	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	if err != nil {
		klog.V(100).Infof("Command %s failed with error: %v\nStdout: %s\nStderr: %s",
			command.String(), err, stdout.String(), stderr.String())

		return err
	}

	return nil
}

// execCommandWithStdout executes the given command and returns the stdout of the command and an error if it fails. It
// captures the stdout and stderr of the command and logs them if the command fails.
func execCommandWithStdout(command *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	if err != nil {
		klog.V(100).Infof("Command %s failed with error: %v\nStdout: %s\nStderr: %s",
			command.String(), err, stdout.String(), stderr.String())

		return "", err
	}

	return stdout.String(), nil
}
