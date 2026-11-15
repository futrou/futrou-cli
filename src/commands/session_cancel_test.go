//go:build !windows

package commands

import (
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"futrou-cli/src/services"
)

func withGrace(t *testing.T, grace time.Duration) *int32 {
	t.Helper()
	oldGrace, oldExit := interruptGrace, exitNow
	var exited int32
	interruptGrace = grace
	exitNow = func(code int) { atomic.StoreInt32(&exited, int32(code)) }
	t.Cleanup(func() { interruptGrace, exitNow = oldGrace, oldExit })
	return &exited
}

// A command that does not finish within the grace period after Ctrl+C is ended.
func TestCancelOnInterruptExitsWhenTheCommandDoesNotFinishInTime(t *testing.T) {
	exited := withGrace(t, 50*time.Millisecond)
	client := services.NewApiClientWithToken("http://127.0.0.1:1", "tok")
	stop := cancelOnInterrupt(client)
	defer stop()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(exited) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(exited) != 130 {
		t.Fatalf("exit code = %d, want 130 after the grace period", atomic.LoadInt32(exited))
	}
}

// A command that finishes within the grace period is not cut off.
func TestCancelOnInterruptDoesNotExitWhenTheCommandFinishes(t *testing.T) {
	exited := withGrace(t, 300*time.Millisecond)
	client := services.NewApiClientWithToken("http://127.0.0.1:1", "tok")
	stop := cancelOnInterrupt(client)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	stop() // the command finished: marked its version failed and returned
	time.Sleep(500 * time.Millisecond)
	if atomic.LoadInt32(exited) != 0 {
		t.Fatal("the process was ended although the command finished in time")
	}
}
