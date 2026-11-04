// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The stdout and stderr layer replaces the process's own descriptors, so it
// is tested in a child: the test binary re-run with childModeEnv set.
const childModeEnv = "CREDKIT_TEST_CHILD"

const stdioTok = "stdio-token-0123456789"

func TestMain(m *testing.M) {
	switch os.Getenv(childModeEnv) {
	case "":
		os.Exit(m.Run())
	case "held":
		childHeld()
		os.Exit(0)
	case "held-parent":
		heldParent()
		os.Exit(0)
	case "fix1-parent":
		fix1ParentMain()
		os.Exit(0)
	case "fix1-appender":
		fix1Appender(os.Getenv("FIX1_FILE"), os.Getenv("FIX1_TAG"))
		os.Exit(0)
	case "fix1-print":
		Start(Declarations{})
		fmt.Println("the answer")
		Exit(0)
	case "fix1-print-3":
		Start(Declarations{})
		fmt.Println("the answer")
		Exit(3)
	case "fix1-stderr":
		Start(Declarations{})
		fmt.Println("the answer")
		fmt.Fprintln(os.Stderr, "a hint nobody sees")
		time.Sleep(50 * time.Millisecond)
		Exit(0)
	case "fix1-mcp-out":
		// A response through MCPOut, which writes to the stream itself.
		Start(Declarations{})
		_, _ = MCPOut().Write([]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n"))
		Exit(0)
	case "fix1-quiet":
		Start(Declarations{})
		Exit(0)
	case "fix1-withheld":
		Start(Declarations{})
		Hold("minted:x", "secret!!")
		fmt.Println("the answer is secret!!")
		Exit(0)
	case "fix1-dropped":
		// A held value with a piece the notice contains: the notice is
		// dropped too.
		Start(Declarations{})
		Hold("minted:x", "zz-withheld-zz")
		fmt.Println("the answer is zz-withheld-zz")
		Exit(0)
	case "fix1-child":
		fix1ChildMain()
		os.Exit(0)
	case "fix1-keeps-stream":
		// A grandchild that outlives this program, its stdout inherited.
		exitWait = 500 * time.Millisecond
		Start(Declarations{})
		fmt.Println("the answer")
		exe, _ := os.Executable()
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), childModeEnv+"=fix1-sleeper")
		cmd.Stdout = os.Stdout
		if err := StartCommand(cmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			Exit(1)
		}
		Exit(0)
	case "fix1-sleeper":
		fix1Sleeper()
		os.Exit(0)
	case "echo":
		// A grandchild that writes to the stdout it inherited.
		fmt.Println(os.Args[len(os.Args)-1])
		os.Exit(0)
	case "stdio":
		childStdio()
	case "flagerr":
		Start(Declarations{CredentialFlags: []string{"--token"}})
		fmt.Fprintf(os.Stderr, "Error: unknown command %q for \"zammad-cli\"\n", os.Args[len(os.Args)-1])
		fmt.Fprintln(os.Stderr, "Run 'zammad-cli --help' for usage.")
		Exit(1)
	case "stdin":
		Start(Declarations{StdinFlags: []string{"--token-stdin"}})
		b, _ := io.ReadAll(os.Stdin)
		fmt.Printf("read %d bytes\n", len(b))
		time.Sleep(50 * time.Millisecond)
		fmt.Printf("token is %s", b)
		Exit(0)
	case "install-fail":
		Start(Declarations{Identifiers: []Identifier{{Source: "config:user"}}})
		fmt.Println("unreachable")
		os.Exit(0)
	case "bigwrite":
		// One write larger than the old 4 MiB unit cap, the held value at
		// its end.
		Start(Declarations{})
		Hold("minted:x", "abcdefgh")
		os.Stdout.Write(append(make([]byte, 8<<20), "abcdefgh\n"...))
		Exit(0)
	case "overbudget":
		// One clean write larger than the unit limit.
		maxUnit = 1 << 20
		Start(Declarations{})
		Hold("minted:x", "abcdefgh")
		os.Stdout.Write(bytes.Repeat([]byte("clean output "), 2<<20/13))
		time.Sleep(50 * time.Millisecond)
		fmt.Println("after")
		Exit(0)
	case "drain":
		Start(Declarations{})
		line := strings.Repeat("x", 99) + "\n"
		for range 10000 {
			os.Stdout.WriteString(line)
		}
		Exit(3)
	}
	kit3Child(os.Getenv(childModeEnv))
	os.Exit(99)
}

func childStdio() {
	Start(Declarations{SecretEnv: []string{"CK_TOKEN"}})
	fmt.Println("clean line one")
	time.Sleep(50 * time.Millisecond)
	fmt.Println("token: " + os.Getenv("CK_TOKEN"))
	time.Sleep(50 * time.Millisecond)
	fmt.Print("split " + stdioTok[:6])
	time.Sleep(50 * time.Millisecond)
	fmt.Println(stdioTok[6:] + " done")
	time.Sleep(50 * time.Millisecond)
	fmt.Fprintln(os.Stderr, "warning: "+strings.ToUpper(stdioTok))
	time.Sleep(50 * time.Millisecond)
	println("runtime write " + stdioTok) // straight to fd 2, around os.Stderr
	time.Sleep(50 * time.Millisecond)
	cachedLog.Print("cached logger " + stdioTok) // a stream captured before Start
	time.Sleep(50 * time.Millisecond)
	// A child that inherits fd 1 writes the token around the kit's API.
	cmd := exec.Command(os.Args[0], "-test.run=^$", "child says "+stdioTok)
	cmd.Env = append(os.Environ(), childModeEnv+"=echo")
	cmd.Stdout = os.Stdout
	cmd.Run()
	time.Sleep(50 * time.Millisecond)
	fmt.Println("clean line two")
	Exit(0)
}

func runChild(t *testing.T, mode, stdin string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^$"}, args...)...)
	cmd.Env = append(os.Environ(), childModeEnv+"="+mode, "CK_TOKEN="+stdioTok)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func TestContract5StdoutStderrAtFileDescriptorLevel(t *testing.T) {
	out, errOut, code := runChild(t, "stdio", "")
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, errOut)
	}
	all := strings.ToLower(out + errOut)
	if strings.Contains(all, strings.ToLower(stdioTok)) || strings.Contains(all, strings.ToLower(stdioTok[6:])) {
		t.Errorf("a credential reached the original streams:\nstdout %q\nstderr %q", out, errOut)
	}
	for _, want := range []string{"clean line one\n", "clean line two\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("clean output %q lost: %q", want, out)
		}
	}
	// One notice each for the env token line, the split token, and the
	// child's line; two on stderr.
	if n := strings.Count(out, Notice(SinkStdout)); n < 3 {
		t.Errorf("stdout carries %d notices, want 3: %q", n, out)
	}
	if n := strings.Count(errOut, Notice(SinkStderr)); n < 3 {
		t.Errorf("stderr carries %d notices, want 3: %q", n, errOut)
	}
}

func TestContract1FlagParseErrorEchoWithheld(t *testing.T) {
	_, errOut, code := runChild(t, "flagerr", "", "--token", "flag-secret-xyz123", "flag-secret-xyz123")
	if code != 1 {
		t.Errorf("exit %d", code)
	}
	if strings.Contains(errOut, "flag-secret-xyz123") || !strings.Contains(errOut, Notice(SinkStderr)) {
		t.Errorf("a flag-parse error echoing a credential flag's value was not withheld: %q", errOut)
	}
}

func TestContract1StdinHeldBeforeOutput(t *testing.T) {
	out, _, _ := runChild(t, "stdin", "piped-secret-7654321\n", "login", "--token-stdin")
	if !strings.Contains(out, "read 21 bytes") {
		t.Errorf("the program did not get stdin: %q", out)
	}
	if strings.Contains(out, "piped-secret") {
		t.Errorf("a credential from stdin was printed: %q", out)
	}
}

func TestContract4InstallFailureExitsBeforeWriting(t *testing.T) {
	out, errOut, code := runChild(t, "install-fail", "")
	if code != ExitInstallFailed || out != "" || errOut != "" {
		t.Errorf("exit %d stdout %q stderr %q; want exit %d and nothing written", code, out, errOut, ExitInstallFailed)
	}
}

func TestExitDrainsEveryByte(t *testing.T) {
	out, _, code := runChild(t, "drain", "")
	if code != 3 || len(out) != 10000*100 {
		t.Errorf("exit %d, %d bytes; want exit 3 and 1000000 bytes", code, len(out))
	}
}

// cachedLog is built at package init, before Start, the way a logger held
// in a package variable is: it keeps the stream it was given.
var cachedLog = log.New(os.Stderr, "", 0)
