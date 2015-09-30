package main

import (
	"fmt"
	"io"
	"math"
	"time"
)

var (
	logTimestampFormat = "Jan 02 15:04:05.000"
)

type timestamped struct {
	w io.Writer
}

// timestamped adds a timestamp to the beginning of log lines
func (t *timestamped) Write(buf []byte) (n int, err error) {
	ts := time.Now()
	runningSecs := ts.Sub(processStart).Seconds()
	secs := int(math.Mod(runningSecs, 60))
	mins := int(runningSecs / 60)
	return fmt.Fprintf(t.w, "%s - %dm%ds %s", ts.In(time.UTC).Format(logTimestampFormat), mins, secs, buf)
}
