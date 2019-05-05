package monitor

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/emitter-io/emitter/internal/async"
	"github.com/emitter-io/stats"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Noop implements Storage contract.
var _ Storage = new(Prometheus)

// Prometheus represents a storage which publishes stats to a statsd sink.
type Prometheus struct {
	reader     stats.Snapshotter               // The reader which reads the snapshot of stats.
	cancel     context.CancelFunc              // The cancellation function.
	gauges     map[string]prometheus.Gauge     // The gauges created
	histograms map[string]prometheus.Histogram // The histograms created
}

// NewPrometheus creates a new prometheus endpoint.
func NewPrometheus(snapshotter stats.Snapshotter, mux *http.ServeMux) *Prometheus {

	mux.Handle("/metrics", promhttp.Handler())

	return &Prometheus{
		reader:     snapshotter,
		gauges:     make(map[string]prometheus.Gauge, 0),
		histograms: make(map[string]prometheus.Histogram, 0),
	}
}

// Name returns the name of the provider.
func (p *Prometheus) Name() string {
	return "prometheus"
}

// Configure configures the storage. The config parameter provided is
// loosely typed, since various storage mechanisms will require different
// configurations.
func (p *Prometheus) Configure(config map[string]interface{}) (err error) {

	// Get the interval from the provider configuration
	interval := defaultInterval
	if v, ok := config["interval"]; ok {
		if i, ok := v.(float64); ok {
			interval = time.Duration(i) * time.Millisecond
		}
	}

	p.cancel = async.Repeat(context.Background(), interval, p.write)

	return
}

// Flush reads and writes stats into this stats sink.
func (p *Prometheus) write() {
	// Create a snapshot and restore it straight away
	snapshot := p.reader.Snapshot()
	m, err := stats.Restore(snapshot)
	if err != nil {
		return
	}

	// Send the node and process-level metrics through
	metrics := m.ToMap()
	p.gauge(metrics, "node.peers")
	p.gauge(metrics, "node.conns")
	p.gauge(metrics, "node.subs")

	for name := range metrics {
		prefix := strings.Split(name, ".")[0]
		switch prefix {
		case "rcv", "send":
			p.histogram(metrics, name)
		}
	}
}

// addGauge creates a gauge and maps it to a metric name
func (p *Prometheus) addGauge(metric string) prometheus.Gauge {
	opts := prometheus.GaugeOpts{
		Name: strings.Replace(metric, ".", "_", -1),
	}
	p.gauges[metric] = promauto.NewGauge(opts)
	return p.gauges[metric]
}

func (p *Prometheus) addHistogram(metric string) prometheus.Histogram {
	opts := prometheus.HistogramOpts{
		Name: strings.Replace(metric, ".", "_", -1),
	}
	p.histograms[metric] = promauto.NewHistogram(opts)
	return p.histograms[metric]
}

// sends the metric as a gauge
func (p *Prometheus) gauge(source map[string]stats.Snapshot, metric string) {
	if v, ok := source[metric]; ok {
		g, ok := p.gauges[metric]
		if !ok {
			g = p.addGauge(metric)
		}
		g.Set(float64(v.Max()))
	}
}

// sends the metric as a histogram
func (p *Prometheus) histogram(source map[string]stats.Snapshot, metric string) {
	if v, ok := source[metric]; ok {
		for _, sample := range v.Sample {
			h, ok := p.histograms[metric]
			if !ok {
				h = p.addHistogram(metric)
			}
			h.Observe(float64(sample))
		}
	}
}

// Close gracefully terminates the storage and ensures that every related
// resource is properly disposed.
func (p *Prometheus) Close() error {
	if p.cancel != nil {
		p.cancel()
	}

	return nil
}
