package sshcommand

import (
	"os"

	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

// SSHCommandResult is the result of executing a command via SSH. Upon
// success the Err wil be nil, upon failure, the Err will be set, and the
// SSHOutput may also be set, depending on the failure.
// Using this instead of returning (string, error) from SSHCommand() makes
// the code much cleaner and simpler when calling SSHCommand() asynchronously
// with a channel.
type SSHCommandResult struct {
	SSHOutput string
	Err       error
}

// Example code to run SSHCommand() asynchronously with a channel.
//	sshChannel := make(chan sshcommand.SSHCommandResult)
//	go func(channel chan sshcommand.SSHCommandResult) {
//		// The command to execute via SSH
//		command := "ls -al /tmp"
//		// The server:port to connect to with SSH
//		sshAddrStr := "192.168.10.3:22"
//		// Execute the command via SSH
//		sshResult := sshcommand.SSHCommand(command,
//			sshAddrStr,
//			"kni",
//			"/home/kni/.ssh/id_rsa")
//		channel <- sshResult
//	}(sshChannel)
//
//	result := <-sshChannel
//	if result.Err != nil {
//		// React accordingly
//	}

// SSHCommand Executes a command on a remote host via ssh.
// For the ssh to work, the ssh privateKey from the test executor has to have
// already been added to the ~/.ssh/authorized_keys file on the remote host.
// The format of sshAddrStr is "192.168.10.3:22".
func SSHCommand(command, sshAddrStr, user, hostPrivateKeyPath string) *SSHCommandResult {
	// The ipsec show command should list 1 line per connection
	klog.V(100).Infof("Execute cmd %s on remote host %s",
		command, sshAddrStr)

	result := new(SSHCommandResult)
	result.SSHOutput = ""
	result.Err = nil

	var (
		keyBuf  []byte
		signer  ssh.Signer
		client  *ssh.Client
		session *ssh.Session
		output  []byte
	)

	keyBuf, result.Err = os.ReadFile(hostPrivateKeyPath)
	if result.Err != nil {
		klog.V(100).Infof("Unable to open private key: %s, err: %v",
			hostPrivateKeyPath, result.Err)

		return result
	}

	signer, result.Err = ssh.ParsePrivateKey(keyBuf)
	if result.Err != nil {
		klog.V(100).Infof("Unable to parse private key: %s, err: %v",
			hostPrivateKeyPath, result.Err)

		return result
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, result.Err = ssh.Dial("tcp", sshAddrStr, config)
	if result.Err != nil {
		klog.V(100).Infof("Failed to dial: %s, err: %v", sshAddrStr, result.Err)

		return result
	}

	session, result.Err = client.NewSession()
	if result.Err != nil {
		klog.V(100).Infof("Failed to create session: %v", result.Err)

		return result
	}
	defer session.Close()

	output, result.Err = session.CombinedOutput(command)
	if result.Err != nil {
		klog.V(100).Infof("Failed to run command: %s, output: %s, err %v",
			command, string(output), result.Err)

		return result
	}

	result.SSHOutput = string(output)
	klog.V(100).Infof("SSH command output: %s", result.SSHOutput)

	return result
}
