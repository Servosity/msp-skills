// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2,
// mcp-http-response-order. Every response the MCP HTTP server writes, its
// refusals included, passes the credential kit.

package threecxblackbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type mcpSandbox struct{ home string }

// newMCPSandbox is an empty home. Not t.TempDir: its path carries the test's name,
// which can share a piece with the tokens these tests hold.
func newMCPSandbox(t *testing.T) mcpSandbox {
	t.Helper()
	home, err := os.MkdirTemp("", "threecx-kit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return mcpSandbox{home: home}
}

func (s mcpSandbox) env(extra ...string) []string {
	return append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + s.home, "USERPROFILE=" + s.home,
		"XDG_CONFIG_HOME=" + s.home, "XDG_DATA_HOME=" + s.home, "XDG_STATE_HOME=" + s.home, "XDG_CACHE_HOME=" + s.home}, extra...)
}

// freePort is a loopback port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// serveMCPHTTP starts the MCP server over HTTP on a loopback port with
// token as its bearer, and returns its address once it accepts connections.
func serveMCPHTTP(t *testing.T, mcp string, s mcpSandbox, token string) (addr string, stderr *bytes.Buffer) {
	t.Helper()
	addr = fmt.Sprintf("127.0.0.1:%d", freePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, mcp, "--transport", "http", "--addr", addr)
	cmd.Env = s.env("THREECX_MCP_HTTP_TOKEN=" + token)
	stderr = &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return addr, stderr
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the MCP HTTP server never listened on %s; stderr:\n%s", addr, stderr.String())
	return "", nil
}

// rawExchange writes request to addr as is and returns every byte the
// server answered with, head included, until it closes the connection.
func rawExchange(t *testing.T, addr, request string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := io.WriteString(c, request); err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(c)
	return string(b)
}

// The server's refusals (no bearer, a foreign Origin, an oversized body, a
// body that cannot be read) answer with fixed text. A bearer token that
// carries that text holds it, so the refusal must pass the kit like every
// other response: Codex's token aaaaaaaaunauthorizedbbbbbbbbbbbbb leaked its
// 12-character piece "unauthorized" through the bearer check's
// "unauthorized\n".
//
// For the reviewer's own token the kit withholds even its replacement (a
// decoding of the notice meets the token's runs of one letter), so it fails
// closed: the connection closes with nothing written. Each refusal is also
// driven with a token of mixed characters, where the kit's refusal goes out
// with the held text taken out.
func TestKitMCPHTTPRefusalsPassTheKit(t *testing.T) {
	t.Parallel()
	mcp := bbMCP
	type refusal struct {
		name, token, piece, request string
		mayFailClosed               bool
	}
	noBearer := func(token string, failClosed bool) refusal {
		return refusal{name: "no bearer, token " + token, token: token, piece: "unauthorized", mayFailClosed: failClosed,
			request: "GET /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAccept: text/event-stream\r\n\r\n"}
	}
	cases := []refusal{
		noBearer("aaaaaaaaunauthorizedbbbbbbbbbbbbb", true), // the reviewer's input
		noBearer("Qz7unauthorizedWm4kP9xL2vB8nR5tY3", false),
		{
			name: "foreign Origin", token: "Hd3forbidden originJw6Ns1Xc4Gt7Yp2", piece: "forbidden",
			request: "POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nOrigin: https://evil.example\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}",
		},
		{
			name: "declared oversized body", token: "Fm2request body too largeKa8Lq5Zr1", piece: "too large",
			request: "POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer Fm2request body too largeKa8Lq5Zr1\r\n" +
				"Content-Type: application/json\r\nContent-Length: 104857600\r\n\r\n",
		},
		{
			name: "a body that cannot be read", token: "Tb6reading request bodyMe9Vx3Cu7Sw4", piece: "reading request",
			request: "POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer Tb6reading request bodyMe9Vx3Cu7Sw4\r\n" +
				"Content-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n{}\r\n0\r\n\r\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newMCPSandbox(t)
			addr, stderr := serveMCPHTTP(t, mcp, s, tc.token)
			resp := rawExchange(t, addr, tc.request)
			if strings.Contains(strings.ToLower(resp), tc.piece) {
				t.Errorf("the refusal carries %q, a piece of the held bearer token:\n%q", tc.piece, resp)
			}
			if !strings.HasPrefix(resp, "HTTP/1.1 4") && !(tc.mayFailClosed && resp == "") {
				t.Errorf("the request was not refused; response:\n%q\nstderr:\n%s", resp, stderr.String())
			}
			if strings.Contains(stderr.String(), tc.token) {
				t.Errorf("stderr carries the bearer token:\n%s", stderr.String())
			}
		})
	}
}

// Refusals now pass the kit, so a refused request reaches it too. Nothing in
// one may mark the kit's acquisition unknown (after which it withholds every
// response): a caller without the bearer sending a body the kit cannot read
// as sent, or a caller with it sending a body that breaks off, must not
// silence the server for the next, ordinary request.
func TestKitMCPHTTPRefusalsLeaveTheServerAnswering(t *testing.T) {
	t.Parallel()
	mcp := bbMCP
	const token = "Kp4ordinaryBearer" + "7Tq2Lx9Vm3Rz8Wn5" // fake; split for the secret scanner
	s := newMCPSandbox(t)
	addr, stderr := serveMCPHTTP(t, mcp, s, token)
	for _, req := range []string{
		"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nContent-Type: text/plain; charset=latin1\r\n" +
			"Content-Encoding: gzip\r\nContent-Length: 4\r\n\r\n\xef\xbb\xbfx",
		"POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer " + token + "\r\n" +
			"Content-Type: application/json\r\nTransfer-Encoding: chunked\r\n\r\nzz\r\n{}\r\n0\r\n\r\n",
	} {
		if resp := rawExchange(t, addr, req); !strings.HasPrefix(resp, "HTTP/1.1 4") {
			t.Fatalf("the request was not refused; response:\n%q\nstderr:\n%s", resp, stderr.String())
		}
	}
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	resp := rawExchange(t, addr, "POST /mcp HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\nAuthorization: Bearer "+token+"\r\n"+
		"Content-Type: application/json\r\nAccept: application/json, text/event-stream\r\n"+fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))+body)
	if !strings.HasPrefix(resp, "HTTP/1.1 200") || !strings.Contains(resp, `"protocolVersion"`) {
		t.Fatalf("an ordinary initialize after the refusals was not answered; response:\n%q\nstderr:\n%s", resp, stderr.String())
	}
}
