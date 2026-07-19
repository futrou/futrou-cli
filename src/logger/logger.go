package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

var (
	defaultStdout io.Writer = os.Stdout
	defaultStderr io.Writer = os.Stderr
	currentStdout io.Writer = defaultStdout
	currentStderr io.Writer = defaultStderr

	traceLogger = log.New(currentStdout, "", 0)
	debugLogger = log.New(currentStdout, "", 0)
	infoLogger  = log.New(currentStdout, "", 0)
	warnLogger  = log.New(currentStdout, "", 0)
	errorLogger = log.New(currentStderr, "", 0)
	fatalLogger = log.New(currentStderr, "", 0)

	logLevel  = INFO
	logFormat = FORMAT_TEXT
	useColors = true
)

const (
	colorReset        = "\033[0m"
	colorRed          = "\033[31m"
	colorGray         = "\033[90m"
	colorWhite        = "\033[97m"
	colorYellow       = "\033[33m"
	colorYellowBright = "\033[93m"
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

var (
	spinnerMu      sync.Mutex
	spinnerStop    chan struct{}
	spinnerWG      sync.WaitGroup
	spinnerRunning bool
	spinnerMessage string
)

const (
	DEBUG = iota
	INFO
	WARN
	ERROR
	FATAL
)

const (
	FORMAT_TEXT = "text"
	FORMAT_JSON = "json"
)

type LogEntry struct {
	Level   string    `json:"level"`
	Time    time.Time `json:"time"`
	Message string    `json:"msg"`
}

func SetOutput(stdout, stderr io.Writer) {
	currentStdout = stdout
	currentStderr = stderr
	traceLogger.SetOutput(stdout)
	debugLogger.SetOutput(stdout)
	infoLogger.SetOutput(stdout)
	warnLogger.SetOutput(stdout)
	errorLogger.SetOutput(stderr)
	fatalLogger.SetOutput(stderr)
}

func SetLogLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		logLevel = DEBUG
	case "info":
		logLevel = INFO
	case "warn":
		logLevel = WARN
	case "error":
		logLevel = ERROR
	case "fatal":
		logLevel = FATAL
	default:
		logLevel = INFO
	}
}

func SetLogFormat(format string) {
	switch strings.ToLower(format) {
	case "json":
		logFormat = FORMAT_JSON
	default:
		logFormat = FORMAT_TEXT
	}
}

func GetLogFormat() string {
	return logFormat
}

// UseColors reports whether ANSI colors should be used for output,
// based on whether stdout is a terminal.
func UseColors() bool {
	return useColors
}

func init() {
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		SetLogLevel(lvl)
	}
	if fmt := os.Getenv("LOG_FORMAT"); fmt != "" {
		SetLogFormat(fmt)
	}
	if file, ok := currentStdout.(*os.File); ok {
		useColors = term.IsTerminal(int(file.Fd()))
	}
}

func Log(l *log.Logger, level, color, format string, args ...interface{}) {
	stopLoader()
	ts := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	if logFormat == FORMAT_JSON {
		entry := LogEntry{Level: strings.ToLower(level), Time: time.Now(), Message: msg}
		data, _ := json.Marshal(entry)
		l.Printf("%s\n", data)
	} else if useColors {
		l.Printf("%s[%s] %s: %s%s\n", color, ts, level, msg, colorReset)
	} else {
		l.Printf("[%s] %s: %s\n", ts, level, msg)
	}
}

func Trace(format string, args ...interface{}) {
	if logLevel <= DEBUG {
		Log(traceLogger, "TRACE", colorGray, format, args...)
	}
}

func Debug(format string, args ...interface{}) {
	if logLevel <= DEBUG {
		Log(debugLogger, "DEBUG", colorGray, format, args...)
	}
}

func Info(format string, args ...interface{}) {
	if logLevel <= INFO {
		Log(infoLogger, "INFO", colorWhite, format, args...)
	}
}

func Warn(format string, args ...interface{}) {
	if logLevel <= WARN {
		Log(warnLogger, "WARN", colorYellow, format, args...)
	}
}

func Error(format string, args ...interface{}) {
	if logLevel <= ERROR {
		Log(errorLogger, "ERROR", colorRed, format, args...)
	}
}

func Fatal(format string, args ...interface{}) {
	Log(fatalLogger, "FATAL", colorRed, format, args...)
	os.Exit(1)
}

// StartLoader begins an animated spinner on the current line, followed by
// message in gray. It does nothing if a spinner is already running.
func StartLoader(message string) {
	spinnerMu.Lock()
	defer spinnerMu.Unlock()
	if spinnerRunning {
		spinnerMessage = message
		return
	}
	if !useColors {
		fmt.Fprintln(currentStdout, message)
		return
	}

	spinnerRunning = true
	spinnerMessage = message
	spinnerStop = make(chan struct{})
	spinnerWG.Add(1)

	go func() {
		defer spinnerWG.Done()
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-spinnerStop:
				return
			case <-ticker.C:
				spinnerMu.Lock()
				msg := spinnerMessage
				spinnerMu.Unlock()
				fmt.Fprintf(currentStdout, "\r\033[K%s%s%s %s%s%s", colorYellowBright, spinnerFrames[frame], colorReset, colorGray, msg, colorReset)
				frame = (frame + 1) % len(spinnerFrames)
			}
		}
	}()
}

// UpdateLoader changes the message shown next to the running spinner. If no
// spinner is running, it behaves like StartLoader.
func UpdateLoader(message string) {
	spinnerMu.Lock()
	running := spinnerRunning
	if running {
		spinnerMessage = message
	}
	spinnerMu.Unlock()
	if !running {
		StartLoader(message)
	}
}

// StopLoader stops the spinner and clears its line.
func StopLoader() {
	stopLoader()
}

// stopLoader stops the spinner if running; called internally before any
// other log output so the spinner line doesn't get clobbered.
func stopLoader() {
	spinnerMu.Lock()
	if !spinnerRunning {
		spinnerMu.Unlock()
		return
	}
	spinnerRunning = false
	stop := spinnerStop
	spinnerMu.Unlock()

	close(stop)
	spinnerWG.Wait()

	if useColors {
		fmt.Fprint(currentStdout, "\r\033[K")
	}
}
