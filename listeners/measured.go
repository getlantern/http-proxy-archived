package listeners

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/getlantern/measured"
)

const (
	rateInterval = 1 * time.Second
)

// MeasuredReportFN is a function that gets called to report stats from the
// measured connection. deltaStats is like stats except that SentTotal and
// RecvTotal are deltas relative to the prior reported stats. final indicates
// whether this is the last call for a connection (i.e. connection has been
// closed).
type MeasuredReportFN func(ctx map[string]interface{}, stats *measured.Stats, deltaStats *measured.Stats,
	final bool)

// Wrapped stateAwareMeasuredListener that generates the wrapped wrapMeasuredConn
type stateAwareMeasuredListener struct {
	net.Listener
	reportInterval time.Duration
	report         MeasuredReportFN
}

func NewMeasuredListener(l net.Listener, reportInterval time.Duration, report MeasuredReportFN) net.Listener {
	return &stateAwareMeasuredListener{l, reportInterval, report}
}

func (l *stateAwareMeasuredListener) Accept() (c net.Conn, err error) {
	c, err = l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	fs := make(chan *measured.Stats)
	wc := &wrapMeasuredConn{
		Conn: measured.Wrap(c, rateInterval, func(mc measured.Conn) {
			fs <- mc.Stats()
		}),
		ctx:        make(map[string]interface{}),
		finalStats: fs,
	}
	sac, _ := c.(WrapConnEmbeddable)
	wc.WrapConnEmbeddable = sac
	go wc.track(l.reportInterval, l.report)
	return wc, nil
}

// Wrapped MeasuredConn that supports OnState
type wrapMeasuredConn struct {
	WrapConnEmbeddable
	measured.Conn
	ctx        map[string]interface{}
	ctxMx      sync.RWMutex
	finalStats chan *measured.Stats
}

func (c *wrapMeasuredConn) track(reportInterval time.Duration, report MeasuredReportFN) {
	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	var priorStats *measured.Stats
	applyStats := func(stats *measured.Stats, final bool) {
		deltaStats := &measured.Stats{}
		// Copy stats
		*deltaStats = *stats
		if priorStats != nil {
			deltaStats.SentTotal -= priorStats.SentTotal
			deltaStats.RecvTotal -= priorStats.RecvTotal
		}
		priorStats = stats
		c.ctxMx.RLock()
		ctx := c.ctx
		c.ctxMx.RUnlock()
		report(ctx, stats, deltaStats, final)
	}

	for {
		select {
		case <-ticker.C:
			applyStats(c.Conn.Stats(), false)
		case stats := <-c.finalStats:
			applyStats(stats, true)
			return
		}
	}
}

func (c *wrapMeasuredConn) OnState(s http.ConnState) {
	if c.WrapConnEmbeddable != nil {
		c.WrapConnEmbeddable.OnState(s)
	}
}

// Responds to the "measured" message type
func (c *wrapMeasuredConn) ControlMessage(msgType string, data interface{}) {
	if msgType == "measured" {
		ctxUpdate := data.(map[string]interface{})
		c.ctxMx.Lock()
		newContext := make(map[string]interface{}, len(c.ctx))
		// Copy context
		for key, value := range c.ctx {
			newContext[key] = value
		}
		// Update context
		for key, value := range ctxUpdate {
			newContext[key] = value
		}
		c.ctx = newContext
		c.ctxMx.Unlock()
	}

	if c.WrapConnEmbeddable != nil {
		// Pass it down too, just in case other wrapper does something with
		c.WrapConnEmbeddable.ControlMessage(msgType, data)
	}
}

func (c *wrapMeasuredConn) Wrapped() net.Conn {
	return c.Conn
}
