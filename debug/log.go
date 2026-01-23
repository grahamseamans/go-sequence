package debug

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	file    *os.File
	mu      sync.Mutex
	enabled bool
)

// Enable starts debug logging to ~/.config/go-sequence/debug.log
func Enable() error {
	mu.Lock()
	defer mu.Unlock()

	if enabled {
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	logPath := homeDir + "/.config/go-sequence/debug.log"

	// Ensure directory exists
	os.MkdirAll(homeDir+"/.config/go-sequence", 0755)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	file = f
	enabled = true

	// Write directly (can't call Log - we hold the mutex)
	ts := time.Now().Format("15:04:05.000")
	fmt.Fprintf(file, "[%s] %-10s %s\n", ts, "debug", "=== Debug logging started ===")
	file.Sync()

	return nil
}

// Disable stops debug logging
func Disable() {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		file.Close()
		file = nil
	}
	enabled = false
}

// Log writes a message to the debug log
func Log(category, format string, args ...any) {
	mu.Lock()
	defer mu.Unlock()

	if !enabled || file == nil {
		return
	}

	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(file, "[%s] %-10s %s\n", ts, category, msg)
	file.Sync() // flush immediately so we see logs even on crash
}

// LogEvery logs only every N calls (use for high-frequency events)
var counters = make(map[string]int)

func LogEvery(n int, category, format string, args ...any) {
	mu.Lock()
	key := category + format
	counters[key]++
	count := counters[key]
	mu.Unlock()

	if count%n == 0 {
		Log(category, format+" (every %d, count=%d)", append(args, n, count)...)
	}
}
