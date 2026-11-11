package metrics

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func RegisterTaskMetrics(conn *sql.DB) {
	prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "transcription_oldest_pending_seconds",
		Help: "Age of the oldest queued or processing task, or zero when there are none.",
	}, func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		var age float64
		if err := conn.QueryRowContext(ctx, `
			SELECT COALESCE(EXTRACT(EPOCH FROM now() - MIN(created_at)), 0)
			FROM tasks WHERE status IN ('queued', 'processing')
		`).Scan(&age); err != nil {
			return math.NaN()
		}
		return age
	}))
}

type WorkerMetrics struct {
	ProviderDuration *prometheus.HistogramVec
	TimeToResult     *prometheus.HistogramVec
	RetriesScheduled prometheus.Counter
}

func NewWorkerMetrics() WorkerMetrics {
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "transcription_provider_duration_seconds",
		Help:    "Time spent in one transcription attempt.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30},
	}, []string{"outcome"})
	completion := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "transcription_time_to_result_seconds",
		Help:    "Time between task creation and a terminal outcome.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300},
	}, []string{"status"})
	retries := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "transcription_retries_scheduled_total",
		Help: "Number of retries committed to the outbox by this worker.",
	})
	prometheus.MustRegister(duration, completion, retries)
	return WorkerMetrics{ProviderDuration: duration, TimeToResult: completion, RetriesScheduled: retries}
}
