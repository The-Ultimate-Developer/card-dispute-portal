// Package metrics owns every Prometheus instrument. Domain packages depend on
// small interfaces they define themselves; *Metrics satisfies all of them, so
// no business package imports Prometheus directly.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	httpRequests  *prometheus.CounterVec
	httpDuration  *prometheus.HistogramVec
	logins        *prometheus.CounterVec
	disputes      *prometheus.CounterVec
	notifications *prometheus.CounterVec
	coreBanking   *prometheus.CounterVec
	circuitState  prometheus.Gauge
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: reg,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total", Help: "HTTP requests by route and status.",
		}, []string{"method", "route", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency by route.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"method", "route"}),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "auth_login_attempts_total", Help: "Sign-in attempts by outcome.",
		}, []string{"outcome"}),
		disputes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "disputes_status_changes_total", Help: "Disputes entering each status.",
		}, []string{"status"}),
		notifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "notifications_delivery_total", Help: "Notification delivery attempts by outcome.",
		}, []string{"topic", "outcome"}),
		coreBanking: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "corebanking_calls_total", Help: "Core banking calls by operation and outcome.",
		}, []string{"operation", "outcome"}),
		circuitState: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "corebanking_circuit_open", Help: "1 while the core banking circuit breaker is open.",
		}),
	}
	reg.MustRegister(m.httpRequests, m.httpDuration, m.logins, m.disputes, m.notifications, m.coreBanking, m.circuitState)
	return m
}

func (m *Metrics) Register(c prometheus.Collector) { m.registry.MustRegister(c) }

func (m *Metrics) ObserveRequest(method, route string, status int, elapsed time.Duration) {
	m.httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.httpDuration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}

func (m *Metrics) ObserveLogin(outcome string)   { m.logins.WithLabelValues(outcome).Inc() }
func (m *Metrics) ObserveDisputeStatus(s string) { m.disputes.WithLabelValues(s).Inc() }
func (m *Metrics) ObserveDelivery(topic, outcome string) {
	m.notifications.WithLabelValues(topic, outcome).Inc()
}
func (m *Metrics) ObserveCoreBankingCall(op, outcome string) {
	m.coreBanking.WithLabelValues(op, outcome).Inc()
}

func (m *Metrics) SetCircuitOpen(open bool) {
	if open {
		m.circuitState.Set(1)
		return
	}
	m.circuitState.Set(0)
}

// Handler serves /metrics. It is disabled without a token and requires the
// token as a bearer credential, because the service is on the public internet.
func (m *Metrics) Handler(token string) http.Handler {
	inner := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || !ok || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.NotFound(w, r)
			return
		}
		inner.ServeHTTP(w, r)
	})
}
