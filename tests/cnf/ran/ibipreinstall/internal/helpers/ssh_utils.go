package helpers

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ibipreinstall/internal/tsparams"
)

const (
	sshSubprocessTimeout = 3 * time.Minute
	sshConnectTimeout    = 10 * time.Second
	sshPort              = "22"
)

// SSHExec executes a command on a remote host via SSH using the Go SSH client.
// An OpenSSH binary is not required in the test image.
func SSHExec(parentCtx context.Context, host, user, sshKeyPath, command string) (string, error) {
	klog.V(tsparams.LogLevel).Infof("Executing on %s@%s: %s", user, host, command)

	ctx, cancel := context.WithTimeout(parentCtx, sshSubprocessTimeout)
	defer cancel()

	client, err := dialSSH(ctx, host, user, sshKeyPath)
	if err != nil {
		return "", wrapSSHError(ctx, err, "")
	}

	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", wrapSSHError(ctx, err, "")
	}

	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), wrapSSHError(ctx, err, string(output))
	}

	return string(output), nil
}

// dialSSH establishes an SSH connection using the Go crypto/ssh client.
// A background goroutine closes the underlying TCP connection when ctx is cancelled.
func dialSSH(ctx context.Context, host, user, sshKeyPath string) (*ssh.Client, error) {
	keyBuf, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open private key %s: %w", sshKeyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(keyBuf)
	if err != nil {
		return nil, fmt.Errorf("unable to parse private key %s: %w", sshKeyPath, err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshConnectTimeout,
	}

	addr := net.JoinHostPort(host, sshPort)
	dialer := net.Dialer{Timeout: sshConnectTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// Unblock handshake and command execution when the context is cancelled
	// or the per-command timeout expires.
	go func() {
		<-ctx.Done()

		_ = conn.Close()
	}()

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()

		return nil, err
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

// wrapSSHError annotates an SSH error with timeout and output context.
func wrapSSHError(ctx context.Context, err error, output string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		if output == "" {
			return fmt.Errorf("ssh timed out after %v: %w", sshSubprocessTimeout, err)
		}

		return fmt.Errorf("ssh timed out after %v: %w, output: %s", sshSubprocessTimeout, err, output)
	}

	if output == "" {
		return fmt.Errorf("ssh failed: %w", err)
	}

	return fmt.Errorf("ssh failed: %w, output: %s", err, output)
}
