package utils

import (
	"io"
	"os"
	"os/exec"
	"runtime"
)

// RunShell runs command with the shell of the platform (sh -c, cmd /C on
// Windows) in dir as a child process. Its output is streamed as it is written
// to stdout and stderr, it reads the terminal's input, and it gets this
// process's environment.
func RunShell(dir, command string, stdout, stderr io.Writer) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}
