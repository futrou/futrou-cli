package logger

import (
	"fmt"
	"sync"
	"time"
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

const (
	colorYellowBright = "\033[93m"
)

var (
	spinnerMu      sync.Mutex
	spinnerStop    chan struct{}
	spinnerWG      sync.WaitGroup
	spinnerRunning bool
	spinnerMessage string
)

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
