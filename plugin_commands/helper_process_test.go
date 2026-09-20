package plugin_commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperProcessFlag = "--plugin-command-helper"

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == helperProcessFlag {
			os.Exit(runPluginCommandHelper(os.Args[i+1:]))
		}
	}
	os.Exit(m.Run())
}

type helperRecord struct {
	Arg0  string   `json:"arg0"`
	Args  []string `json:"args"`
	Env   []string `json:"env"`
	Cwd   string   `json:"cwd"`
	Stdin string   `json:"stdin"`
}

func runPluginCommandHelper(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing helper mode")
		return 2
	}
	switch args[0] {
	case "record":
		if len(args) < 2 {
			return 2
		}
		cwd, err := os.Getwd()
		if err != nil {
			return 3
		}
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return 4
		}
		encoded, err := json.Marshal(helperRecord{Arg0: os.Args[0], Args: args[2:], Env: os.Environ(), Cwd: cwd, Stdin: string(stdin)})
		if err != nil {
			return 5
		}
		if err := os.WriteFile(filepath.Join(args[1], "record.json"), encoded, 0o600); err != nil {
			return 6
		}
		fmt.Fprintln(os.Stdout, "stdout-record")
		fmt.Fprintln(os.Stderr, "stderr-record")
		return 0
	case "tail":
		if len(args) != 2 {
			return 2
		}
		n, err := strconv.Atoi(args[1])
		if err != nil {
			return 3
		}
		_, _ = io.WriteString(os.Stdout, "\x1b[31m\x00")
		_, _ = io.WriteString(os.Stdout, strings.Repeat("x", n))
		_, _ = io.WriteString(os.Stdout, "\x1b[0m\n")
		return 0
	case "spawn-descendant", "spawn-scrubbed-descendant":
		if len(args) != 2 {
			return 2
		}
		child := exec.Command(os.Args[0], helperProcessFlag, "descendant", args[1])
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if args[0] == "spawn-scrubbed-descendant" {
			child.Env = []string{"PATH=" + os.Getenv("PATH")}
		}
		if err := child.Start(); err != nil {
			return 3
		}
		if err := os.WriteFile(filepath.Join(args[1], "descendant.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			return 4
		}
		fmt.Fprintln(os.Stdout, "parent-started-descendant")
		for {
			time.Sleep(time.Second)
		}
	case "spawn-detached-descendant":
		if len(args) != 2 {
			return 2
		}
		child := exec.Command(os.Args[0], helperProcessFlag, "descendant", args[1])
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		configureDetachedProcess(child)
		if err := child.Start(); err != nil {
			return 3
		}
		if err := os.WriteFile(filepath.Join(args[1], "descendant.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
			return 4
		}
		fmt.Fprintln(os.Stdout, "parent-left-detached-descendant")
		return 0
	case "descendant":
		if len(args) != 2 {
			return 2
		}
		time.Sleep(700 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(args[1], "late-write"), []byte("descendant survived"), 0o600)
		fmt.Fprintln(os.Stdout, "late-descendant-output")
		for {
			time.Sleep(time.Second)
		}
	case "exit":
		if len(args) != 2 {
			return 2
		}
		code, err := strconv.Atoi(args[1])
		if err != nil {
			return 3
		}
		return code
	case "write", "write-exit":
		if len(args) != 3 {
			return 2
		}
		n, err := strconv.Atoi(args[2])
		if err != nil {
			return 3
		}
		if err := os.WriteFile(filepath.Join(args[1], "payload.bin"), []byte(strings.Repeat("q", n)), 0o600); err != nil {
			return 4
		}
		fmt.Fprintln(os.Stdout, "wrote payload")
		if args[0] == "write-exit" {
			return 0
		}
		for {
			time.Sleep(time.Second)
		}
	case "sleep":
		for {
			time.Sleep(time.Second)
		}
	case "sleep-ms":
		if len(args) != 2 {
			return 2
		}
		delay, err := time.ParseDuration(args[1] + "ms")
		if err != nil {
			return 3
		}
		time.Sleep(delay)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown helper mode")
		return 2
	}
}

func helperExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	target, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}
