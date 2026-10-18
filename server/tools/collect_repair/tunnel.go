package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"
)

// startTunnel owns its foreground SSH process. Nothing is exposed on LAN.
func startTunnel(ctx context.Context, host, identity string, dbPort, redisPort int) (func(), error) {
	for _, port := range []int{dbPort, redisPort} {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			return nil, fmt.Errorf("local forwarding port %d is occupied", port)
		}
		l.Close()
	}
	args := []string{"-N", "-T", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-L", fmt.Sprintf("127.0.0.1:%d:10.3.0.10:5432", dbPort), "-L", fmt.Sprintf("127.0.0.1:%d:10.3.0.12:6379", redisPort)}
	if endpoint := os.Getenv("COLLECT_REPAIR_BOT_API_FORWARD"); endpoint != "" {
		args = append(args, "-L", endpoint)
	}
	if endpoint := os.Getenv("COLLECT_REPAIR_BOT_FILE_FORWARD"); endpoint != "" {
		args = append(args, "-L", endpoint)
	}
	if proxy := os.Getenv("COLLECT_REPAIR_SSH_PROXY_COMMAND"); proxy != "" {
		args = append(args, "-o", "ProxyCommand="+proxy)
	}
	if identity != "" {
		args = append(args, "-i", identity)
	}
	args = append(args, host)
	tunnelCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(tunnelCtx, "ssh", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	cleanup := func() { cancel(); <-done }
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			cancel()
			return nil, fmt.Errorf("SSH tunnel stopped: %v", err)
		case <-ctx.Done():
			cleanup()
			return nil, ctx.Err()
		case <-deadline.C:
			cleanup()
			return nil, fmt.Errorf("SSH tunnel startup timed out")
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", dbPort), time.Second)
			if err == nil {
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				// PostgreSQL SSLRequest proves the remote endpoint responds.
				_, writeErr := conn.Write([]byte{0, 0, 0, 8, 4, 210, 22, 47})
				reply := make([]byte, 1)
				_, readErr := io.ReadFull(conn, reply)
				conn.Close()
				if writeErr == nil && readErr == nil && (reply[0] == 'S' || reply[0] == 'N') {
					return cleanup, nil
				}
			}
		}
	}
}
