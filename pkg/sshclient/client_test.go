package sshclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func testClient(t *testing.T) *ssh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, cfg)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			ch, reqs, err := incoming.Accept()
			if err != nil {
				return
			}
			go func() {
				defer ch.Close()
				for req := range reqs {
					if req.Type != "exec" {
						req.Reply(false, nil)
						continue
					}
					var payload struct{ Command string }
					ssh.Unmarshal(req.Payload, &payload)
					req.Reply(true, nil)
					if payload.Command == "wait" {
						<-stop
						return
					}
					input, _ := io.ReadAll(ch)
					ch.Write(input)
					ch.Stderr().Write([]byte("stderr"))
					code := uint32(0)
					if payload.Command == "fail" {
						code = 7
					}
					ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
					return
				}
			}()
		}
	}()
	client, err := Dial(Config{Address: listener.Addr().String(), User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}
func TestRunInputOutputAndExitCode(t *testing.T) {
	client := testClient(t)
	for _, command := range []string{"echo", "fail"} {
		result, err := Run(context.Background(), client, command, []byte("stdout"))
		if !strings.Contains(result.Output, "stdout") || !strings.Contains(result.Output, "stderr") {
			t.Fatalf("missing streams: %+v", result)
		}
		if command == "echo" && (err != nil || result.ExitCode != 0) {
			t.Fatalf("success: %+v %v", result, err)
		}
		if command == "fail" && (err == nil || result.ExitCode != 7) {
			t.Fatalf("failure: %+v %v", result, err)
		}
	}
}
func TestRunCancellation(t *testing.T) {
	client := testClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := Run(ctx, client, "wait", nil)
	if !errors.Is(err, context.DeadlineExceeded) || result.ExitCode != -1 {
		t.Fatalf("timeout: %+v %v", result, err)
	}
}

func TestRunSessionFailureIsNotSuccess(t *testing.T) {
	client := testClient(t)
	_ = client.Close()
	result, err := Run(context.Background(), client, "echo", nil)
	if err == nil || result.ExitCode != 1 {
		t.Fatalf("session failure: %+v %v", result, err)
	}
}
