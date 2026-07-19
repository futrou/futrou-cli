package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// withCapture redirects logger output to buffers for the duration of the
// test, restores the previous state afterward, and disables colors so
// assertions can match plain text.
func withCapture(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}

	prevStdout, prevStderr := currentStdout, currentStderr
	prevColors := useColors
	prevLevel := logLevel
	prevFormat := logFormat

	SetOutput(stdout, stderr)
	useColors = false

	t.Cleanup(func() {
		SetOutput(prevStdout, prevStderr)
		useColors = prevColors
		logLevel = prevLevel
		logFormat = prevFormat
	})

	return stdout, stderr
}

func TestSetOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	SetOutput(&out, &errOut)
	t.Cleanup(func() { SetOutput(defaultStdout, defaultStderr) })

	SetLogLevel("info")
	Info("hello")
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("expected stdout to contain %q, got %q", "hello", out.String())
	}

	Error("boom")
	if !strings.Contains(errOut.String(), "boom") {
		t.Errorf("expected stderr to contain %q, got %q", "boom", errOut.String())
	}
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"warn", WARN},
		{"error", ERROR},
		{"fatal", FATAL},
		{"bogus", INFO},
		{"", INFO},
	}
	for _, tt := range tests {
		SetLogLevel(tt.input)
		if logLevel != tt.want {
			t.Errorf("SetLogLevel(%q): got level %d, want %d", tt.input, logLevel, tt.want)
		}
	}
	SetLogLevel("info")
}

func TestSetLogFormat(t *testing.T) {
	SetLogFormat("json")
	if GetLogFormat() != FORMAT_JSON {
		t.Errorf("expected format json, got %s", GetLogFormat())
	}

	SetLogFormat("text")
	if GetLogFormat() != FORMAT_TEXT {
		t.Errorf("expected format text, got %s", GetLogFormat())
	}

	SetLogFormat("bogus")
	if GetLogFormat() != FORMAT_TEXT {
		t.Errorf("expected unknown format to fall back to text, got %s", GetLogFormat())
	}
}

func TestUseColors(t *testing.T) {
	prev := useColors
	defer func() { useColors = prev }()

	useColors = true
	if !UseColors() {
		t.Error("expected UseColors() to return true")
	}
	useColors = false
	if UseColors() {
		t.Error("expected UseColors() to return false")
	}
}

func TestTrace(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("debug")
	Trace("trace %s", "msg")
	if !strings.Contains(out.String(), "TRACE") || !strings.Contains(out.String(), "trace msg") {
		t.Errorf("expected TRACE log with message, got %q", out.String())
	}
}

func TestTrace_belowLevel(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("info")
	Trace("should not appear")
	if out.String() != "" {
		t.Errorf("expected no output at info level, got %q", out.String())
	}
}

func TestDebug(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("debug")
	Debug("debug %s", "msg")
	if !strings.Contains(out.String(), "DEBUG") || !strings.Contains(out.String(), "debug msg") {
		t.Errorf("expected DEBUG log with message, got %q", out.String())
	}
}

func TestDebug_belowLevel(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("info")
	Debug("should not appear")
	if out.String() != "" {
		t.Errorf("expected no output at info level, got %q", out.String())
	}
}

func TestInfo(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("info")
	Info("info %s", "msg")
	if !strings.Contains(out.String(), "INFO") || !strings.Contains(out.String(), "info msg") {
		t.Errorf("expected INFO log with message, got %q", out.String())
	}
}

func TestInfo_belowLevel(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("warn")
	Info("should not appear")
	if out.String() != "" {
		t.Errorf("expected no output at warn level, got %q", out.String())
	}
}

func TestWarn(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("warn")
	Warn("warn %s", "msg")
	if !strings.Contains(out.String(), "WARN") || !strings.Contains(out.String(), "warn msg") {
		t.Errorf("expected WARN log with message, got %q", out.String())
	}
}

func TestWarn_belowLevel(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("error")
	Warn("should not appear")
	if out.String() != "" {
		t.Errorf("expected no output at error level, got %q", out.String())
	}
}

func TestError(t *testing.T) {
	_, errOut := withCapture(t)
	SetLogLevel("error")
	Error("error %s", "msg")
	if !strings.Contains(errOut.String(), "ERROR") || !strings.Contains(errOut.String(), "error msg") {
		t.Errorf("expected ERROR log with message, got %q", errOut.String())
	}
}

func TestError_belowLevel(t *testing.T) {
	_, errOut := withCapture(t)
	SetLogLevel("fatal")
	Error("should not appear")
	if errOut.String() != "" {
		t.Errorf("expected no output at fatal level, got %q", errOut.String())
	}
}

func TestLog_jsonFormat(t *testing.T) {
	out, _ := withCapture(t)
	SetLogLevel("info")
	SetLogFormat("json")
	t.Cleanup(func() { SetLogFormat("text") })

	Info("hello %s", "world")

	var entry LogEntry
	if err := json.Unmarshal(out.Bytes(), &entry); err != nil {
		t.Fatalf("expected valid JSON output, got %q: %v", out.String(), err)
	}
	if entry.Level != "info" {
		t.Errorf("expected level 'info', got %q", entry.Level)
	}
	if entry.Message != "hello world" {
		t.Errorf("expected message 'hello world', got %q", entry.Message)
	}
	if entry.Time.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestLog_colors(t *testing.T) {
	var out bytes.Buffer
	SetOutput(&out, &out)
	t.Cleanup(func() { SetOutput(defaultStdout, defaultStderr) })

	prevColors := useColors
	useColors = true
	t.Cleanup(func() { useColors = prevColors })

	SetLogLevel("info")
	SetLogFormat("text")
	Info("colored")

	if !strings.Contains(out.String(), colorWhite) || !strings.Contains(out.String(), colorReset) {
		t.Errorf("expected ANSI color codes in output, got %q", out.String())
	}
}

// Fatal calls os.Exit and is exercised via a subprocess in most codebases;
// here we only verify it logs before exiting would occur, by checking the
// Log call path shares the same formatting as the other levels (covered
// above). A direct test of Fatal's os.Exit(1) is intentionally omitted.

func TestStartLoader_and_StopLoader(t *testing.T) {
	var out bytes.Buffer
	SetOutput(&out, &out)
	t.Cleanup(func() { SetOutput(defaultStdout, defaultStderr) })

	prevColors := useColors
	useColors = true
	t.Cleanup(func() { useColors = prevColors })

	StartLoader("working...")
	time.Sleep(150 * time.Millisecond)
	StopLoader()

	got := out.String()
	if !strings.Contains(got, "working...") {
		t.Errorf("expected spinner output to contain message, got %q", got)
	}
	if spinnerRunning {
		t.Error("expected spinner to be stopped")
	}
}

func TestStartLoader_noColors(t *testing.T) {
	out, _ := withCapture(t) // useColors = false

	StartLoader("no color loader")
	t.Cleanup(StopLoader)

	if !strings.Contains(out.String(), "no color loader") {
		t.Errorf("expected plain message when colors disabled, got %q", out.String())
	}
	if spinnerRunning {
		t.Error("expected no spinner goroutine when colors are disabled")
	}
}

func TestUpdateLoader_whileRunning(t *testing.T) {
	var out bytes.Buffer
	SetOutput(&out, &out)
	t.Cleanup(func() { SetOutput(defaultStdout, defaultStderr) })

	prevColors := useColors
	useColors = true
	t.Cleanup(func() { useColors = prevColors })

	StartLoader("first")
	time.Sleep(50 * time.Millisecond)
	UpdateLoader("second")
	time.Sleep(150 * time.Millisecond)
	StopLoader()

	if !strings.Contains(out.String(), "second") {
		t.Errorf("expected updated message in output, got %q", out.String())
	}
}

func TestUpdateLoader_startsWhenNotRunning(t *testing.T) {
	out, _ := withCapture(t) // useColors = false

	UpdateLoader("started via update")
	t.Cleanup(StopLoader)

	if !strings.Contains(out.String(), "started via update") {
		t.Errorf("expected UpdateLoader to start a loader when none running, got %q", out.String())
	}
}

func TestStopLoader_whenNotRunning(t *testing.T) {
	withCapture(t)
	// Should be a no-op, not panic or block.
	StopLoader()
	StopLoader()
}

func TestLog_stopsRunningLoader(t *testing.T) {
	var out bytes.Buffer
	SetOutput(&out, &out)
	t.Cleanup(func() { SetOutput(defaultStdout, defaultStderr) })

	prevColors := useColors
	useColors = true
	t.Cleanup(func() { useColors = prevColors })

	SetLogLevel("info")
	SetLogFormat("text")

	StartLoader("in progress")
	time.Sleep(100 * time.Millisecond)
	Info("done")

	if spinnerRunning {
		t.Error("expected Info() to stop the running spinner")
	}
	if !strings.Contains(out.String(), "done") {
		t.Errorf("expected log message after spinner stop, got %q", out.String())
	}
}
