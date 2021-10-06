package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/appdir"
	"github.com/getlantern/golog"
	"github.com/getlantern/rotator"
)

const (
	logTimestampFormat = "Jan 02 15:04:05.000"
)

var (
	log          = golog.LoggerFor("flashlight.logging")
	logdir       = logDir()
	processStart = time.Now()

	logFile *rotator.SizeRotator

	errorOut io.Writer
	debugOut io.Writer
	outLock  sync.Mutex

	duplicates = make(map[string]bool)
	dupLock    sync.Mutex
)

// timestamped adds a timestamp to the beginning of log lines
type timestamped struct {
	io.Writer
}

func logDir() string {
	if runtime.GOOS == "darwin" {
		return appdir.Logs("http-proxy")
	}

	return "/var/log/http-proxy"
}

func (t timestamped) Write(p []byte) (int, error) {
	// Write in single operation to prevent different log items from interleaving
	return io.WriteString(t.Writer, time.Now().In(time.UTC).Format(logTimestampFormat)+" "+string(p))
}

func Init(instanceId string, version string, revisionDate string) error {
	outLock.Lock()
	defer outLock.Unlock()

	log.Tracef("Placing logs in %v", logdir)
	if _, err := os.Stat(logdir); err != nil {
		if os.IsNotExist(err) {
			// Create log dir
			if err := os.MkdirAll(logdir, 0755); err != nil {
				return fmt.Errorf("Unable to create logdir at %s: %s", logdir, err)
			}
		}
	}
	logFile = rotator.NewSizeRotator(filepath.Join(logdir, "proxy.log"))
	// Set log files to 4 MB
	logFile.RotationSize = 4 * 1024 * 1024
	// Keep up to 5 log files
	logFile.MaxRotation = 5

	// Loggly has its own timestamp so don't bother adding it in message,
	// moreover, golog always write each line in whole, so we need not to care about line breaks.
	errorOut = timestamped{NonStopWriter(os.Stderr, logFile)}
	debugOut = timestamped{NonStopWriter(os.Stdout, logFile)}
	golog.SetOutputs(errorOut, debugOut)

	return nil
}

// Flush forces output flushing if the output is flushable
func Flush() {
	outLock.Lock()
	defer outLock.Unlock()

	if errorOut != nil {
		if output, ok := errorOut.(flushable); ok {
			output.flush()
		}
	}
	if debugOut != nil {
		if output, ok := debugOut.(flushable); ok {
			output.flush()
		}
	}
}

func Close() error {
	golog.ResetOutputs()
	return logFile.Close()
}

func isDuplicate(msg string) bool {
	dupLock.Lock()
	defer dupLock.Unlock()

	if duplicates[msg] {
		return true
	}

	// Implement a crude cap on the size of the map
	if len(duplicates) < 1000 {
		duplicates[msg] = true
	}

	return false
}

// flushable interface describes writers that can be flushed
type flushable interface {
	flush()
	Write(p []byte) (n int, err error)
}

type nonStopWriter struct {
	writers []io.Writer
}

// NonStopWriter creates a writer that duplicates its writes to all the
// provided writers, even if errors encountered while writting.
func NonStopWriter(writers ...io.Writer) io.Writer {
	w := make([]io.Writer, len(writers))
	copy(w, writers)
	return &nonStopWriter{w}
}

// Write implements the method from io.Writer.
// It never fails and always return the length of bytes passed in
func (t *nonStopWriter) Write(p []byte) (int, error) {
	for _, w := range t.writers {
		// intentionally not checking for errors
		_, _ = w.Write(p)
	}
	return len(p), nil
}

// flush forces output of the writers that may provide this functionality.
func (t *nonStopWriter) flush() {
	for _, w := range t.writers {
		if w, ok := w.(flushable); ok {
			w.flush()
		}
	}
}
