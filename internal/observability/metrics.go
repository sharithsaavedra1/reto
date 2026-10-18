package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default latency buckets in seconds for HTTP duration histogram
var DefaultDurationBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0,
}

// REDMetricsRegistry manages Rate, Errors, and Duration metrics for HTTP endpoints.
type REDMetricsRegistry struct {
	mu sync.RWMutex

	// Rate & Errors: request counters keyed by "method|path|status"
	requestsTotal       map[string]uint64
	requestErrorsTotal  map[string]uint64

	// Duration: latency histograms keyed by "method|path"
	durationBuckets map[string][]uint64
	durationSum     map[string]float64
	durationCount   map[string]uint64

	// Gauges
	requestsInFlight int64
	buckets          []float64
}

// NewREDMetricsRegistry initializes a thread-safe registry with default duration buckets.
func NewREDMetricsRegistry() *REDMetricsRegistry {
	return &REDMetricsRegistry{
		requestsTotal:      make(map[string]uint64),
		requestErrorsTotal: make(map[string]uint64),
		durationBuckets:    make(map[string][]uint64),
		durationSum:        make(map[string]float64),
		durationCount:      make(map[string]uint64),
		buckets:            DefaultDurationBuckets,
	}
}

// IncInFlight increments the in-flight requests gauge.
func (r *REDMetricsRegistry) IncInFlight() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestsInFlight++
}

// DecInFlight decrements the in-flight requests gauge.
func (r *REDMetricsRegistry) DecInFlight() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestsInFlight--
}

// RecordRequest records a completed HTTP request for RED metrics.
func (r *REDMetricsRegistry) RecordRequest(method, path string, statusCode int, duration time.Duration) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = sanitizePath(path)
	statusStr := strconv.Itoa(statusCode)
	sec := duration.Seconds()

	keyWithStatus := fmt.Sprintf("%s|%s|%s", method, path, statusStr)
	keyWithoutStatus := fmt.Sprintf("%s|%s", method, path)

	r.mu.Lock()
	defer r.mu.Unlock()

	// 1. Rate (Total requests counter)
	r.requestsTotal[keyWithStatus]++

	// 2. Errors (Total error requests counter, HTTP 4xx and 5xx)
	if statusCode >= 400 {
		r.requestErrorsTotal[keyWithStatus]++
	}

	// 3. Duration (Histogram recording)
	bucketCounts, exists := r.durationBuckets[keyWithoutStatus]
	if !exists {
		bucketCounts = make([]uint64, len(r.buckets))
		r.durationBuckets[keyWithoutStatus] = bucketCounts
	}

	for i, b := range r.buckets {
		if sec <= b {
			bucketCounts[i]++
		}
	}
	r.durationSum[keyWithoutStatus] += sec
	r.durationCount[keyWithoutStatus]++
}

// GetSummary returns high-level metrics for quick inspection.
func (r *REDMetricsRegistry) GetSummary() (total uint64, errors uint64, errorRate float64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, count := range r.requestsTotal {
		total += count
	}
	for _, count := range r.requestErrorsTotal {
		errors += count
	}
	if total > 0 {
		errorRate = (float64(errors) / float64(total)) * 100.0
	}
	return total, errors, errorRate
}

// ExportPrometheus formats all RED metrics in standard Prometheus exposition text format.
func (r *REDMetricsRegistry) ExportPrometheus() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder

	// Header comments and type declarations
	sb.WriteString("# HELP http_requests_total Total number of HTTP requests processed (RED Rate metric)\n")
	sb.WriteString("# TYPE http_requests_total counter\n")

	keys := make([]string, 0, len(r.requestsTotal))
	for k := range r.requestsTotal {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) == 3 {
			method, path, status := parts[0], parts[1], parts[2]
			count := r.requestsTotal[k]
			fmt.Fprintf(&sb, "http_requests_total{method=%q,path=%q,status=%q} %d\n", method, path, status, count)
		}
	}

	sb.WriteString("\n# HELP http_requests_errors_total Total number of HTTP requests returning errors (status >= 400) (RED Errors metric)\n")
	sb.WriteString("# TYPE http_requests_errors_total counter\n")

	errKeys := make([]string, 0, len(r.requestErrorsTotal))
	for k := range r.requestErrorsTotal {
		errKeys = append(errKeys, k)
	}
	sort.Strings(errKeys)

	for _, k := range errKeys {
		parts := strings.Split(k, "|")
		if len(parts) == 3 {
			method, path, status := parts[0], parts[1], parts[2]
			count := r.requestErrorsTotal[k]
			fmt.Fprintf(&sb, "http_requests_errors_total{method=%q,path=%q,status=%q} %d\n", method, path, status, count)
		}
	}

	sb.WriteString("\n# HELP http_request_duration_seconds HTTP request latencies in seconds (RED Duration metric)\n")
	sb.WriteString("# TYPE http_request_duration_seconds histogram\n")

	durKeys := make([]string, 0, len(r.durationBuckets))
	for k := range r.durationBuckets {
		durKeys = append(durKeys, k)
	}
	sort.Strings(durKeys)

	for _, k := range durKeys {
		parts := strings.Split(k, "|")
		if len(parts) == 2 {
			method, path := parts[0], parts[1]
			bucketCounts := r.durationBuckets[k]
			sum := r.durationSum[k]
			count := r.durationCount[k]

			for i, le := range r.buckets {
				fmt.Fprintf(&sb, "http_request_duration_seconds_bucket{method=%q,path=%q,le=\"%g\"} %d\n",
					method, path, le, bucketCounts[i])
			}
			fmt.Fprintf(&sb, "http_request_duration_seconds_bucket{method=%q,path=%q,le=\"+Inf\"} %d\n",
				method, path, count)
			fmt.Fprintf(&sb, "http_request_duration_seconds_sum{method=%q,path=%q} %.6f\n",
				method, path, sum)
			fmt.Fprintf(&sb, "http_request_duration_seconds_count{method=%q,path=%q} %d\n",
				method, path, count)
		}
	}

	sb.WriteString("\n# HELP http_requests_in_flight Current number of HTTP requests being served\n")
	sb.WriteString("# TYPE http_requests_in_flight gauge\n")
	fmt.Fprintf(&sb, "http_requests_in_flight %d\n", r.requestsInFlight)

	return sb.String()
}

// MetricsHandler returns an http.HandlerFunc that serves metrics at /metrics
func (r *REDMetricsRegistry) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.ExportPrometheus()))
	}
}

// sanitizePath replaces dynamic UUIDs with parameterized tokens (e.g. {id}) to avoid metric label explosion.
func sanitizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return "/"
	}

	parts := strings.Split(p, "/")
	for i, part := range parts {
		if isUUIDLike(part) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func isUUIDLike(s string) bool {
	if len(s) == 36 && strings.Count(s, "-") == 4 {
		return true
	}
	return false
}
