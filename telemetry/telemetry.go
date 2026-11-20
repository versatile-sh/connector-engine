// Package telemetry serves the Prometheus metrics of the engine: calls by outcome, approvals by state, call latency, worker failures and outbox lag.
package telemetry

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/agents-control/connector-engine/gateway"
	"github.com/agents-control/connector-engine/port"
	"github.com/agents-control/connector-engine/service"
)

// Metrics holds the registry and the instruments; labels are the tenant id and a closed set of outcomes or states, never an argument or a result.
type Metrics struct {
	registry  *prometheus.Registry
	calls     *prometheus.CounterVec
	approvals *prometheus.CounterVec
	latency   *prometheus.HistogramVec
	failures  prometheus.Counter
}

// New registers the catalogue; lag reads the age of the oldest undelivered outbox event at each scrape.
func New(outbox port.Outbox, now func() time.Time) *Metrics {
	m := &Metrics{registry: prometheus.NewRegistry()}
	m.calls = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "connector_calls_total", Help: "Agent tool calls by outcome."}, []string{"tenant", "outcome"})
	m.approvals = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "connector_approvals_total", Help: "Approval records by the state they reached."}, []string{"tenant", "state"})
	m.latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "connector_gateway_call_duration_seconds", Help: "Time the gateway took to answer a tool call.",
		Buckets: prometheus.ExponentialBuckets(0.005, 4, 8)}, []string{"tenant"})
	m.failures = prometheus.NewCounter(prometheus.CounterOpts{Name: "connector_worker_sweep_failures_total", Help: "Worker sweeps and approved executions that failed or panicked."})
	lag := prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "connector_outbox_lag_seconds", Help: "Age of the oldest event not yet delivered."}, func() float64 {
		pending, err := outbox.Pending(context.Background(), 1)
		if err != nil || len(pending) == 0 {
			return 0
		}
		return max(0, now().Sub(pending[0].Time).Seconds())
	})
	m.registry.MustRegister(m.calls, m.approvals, m.latency, m.failures, lag)
	return m
}

// Handler serves the metrics in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// WorkerFailed counts one failed or panicked worker sweep or approved execution.
func (m *Metrics) WorkerFailed() { m.failures.Inc() }

type metered struct {
	gateway.Core
	m *Metrics
}

// Core counts each call by outcome and times it; every other method is the wrapped core's.
func (m *Metrics) Core(next gateway.Core) gateway.Core { return metered{Core: next, m: m} }

func (c metered) Call(ctx context.Context, p port.Principal, req service.CallRequest) (service.Outcome, error) {
	start := time.Now()
	out, err := c.Core.Call(ctx, p, req)
	tenant := string(p.Claims.Tenant)
	outcome := string(out.Status)
	if err != nil {
		outcome = "failed"
	}
	c.m.calls.WithLabelValues(tenant, outcome).Inc()
	c.m.latency.WithLabelValues(tenant).Observe(time.Since(start).Seconds())
	return out, err
}

type counting struct {
	next port.EventSink
	m    *Metrics
}

// Sink counts the approval events that pass through it and forwards every event.
func (m *Metrics) Sink(next port.EventSink) port.EventSink { return counting{next: next, m: m} }

func approvalState(e port.Event) string {
	suffix, ok := strings.CutPrefix(e.Type, "connector.approval.")
	switch {
	case !ok:
		return ""
	case suffix == "requested":
		return "pending"
	case suffix == "decided":
		if d, _ := e.Data["decision"].(string); d == "deny" {
			return "denied"
		}
		return "approved"
	case suffix == "expired" || suffix == "cancelled":
		return suffix
	}
	return ""
}

func (c counting) Deliver(ctx context.Context, events []port.Event) error {
	err := c.next.Deliver(ctx, events)
	if err == nil {
		for _, e := range events {
			if state := approvalState(e); state != "" {
				c.m.approvals.WithLabelValues(string(e.Tenant), state).Inc()
			}
		}
	}
	return err
}
