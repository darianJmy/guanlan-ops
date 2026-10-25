package sshclient

import (
	"bytes"
	"context"
	"errors"
	"golang.org/x/crypto/ssh"
	"guanlan/pkg/syncbuffer"
	"time"
)

type Config struct {
	Address, User   string
	Auth            []ssh.AuthMethod
	HostKeyCallback ssh.HostKeyCallback
	Timeout         time.Duration
}

func Dial(cfg Config) (*ssh.Client, error) {
	return ssh.Dial("tcp", cfg.Address, &ssh.ClientConfig{User: cfg.User, Auth: cfg.Auth, HostKeyCallback: cfg.HostKeyCallback, Timeout: cfg.Timeout})
}
func PrivateKey(key, passphrase []byte) (ssh.AuthMethod, error) {
	var signer ssh.Signer
	var err error
	if len(passphrase) > 0 {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

type Result struct {
	Output   string
	ExitCode int
}

// Run leaves ownership of client with the caller and closes the command session.
func Run(ctx context.Context, client *ssh.Client, command string, input []byte) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{ExitCode: -1}, err
	}
	session, err := client.NewSession()
	if err != nil {
		return Result{ExitCode: 1}, err
	}
	defer session.Close()
	var output syncbuffer.Buffer
	session.Stdin, session.Stdout, session.Stderr = bytes.NewReader(input), &output, &output
	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			code = 1
			var exitErr *ssh.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitStatus()
			}
		}
		return Result{Output: output.String(), ExitCode: code}, err
	case <-ctx.Done():
		_ = session.Close()
		return Result{Output: output.String(), ExitCode: -1}, ctx.Err()
	}
}
