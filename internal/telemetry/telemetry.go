package telemetry

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type Exporter struct {
	reader   *sdkmetric.PeriodicReader
	meter    metric.Meter
	memStats runtime.MemStats
	lastGC   uint32
}

func NewExporter(endpoint, serviceName string) (*Exporter, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("telemetry endpoint is required")
	}

	exporter, err := otlpmetrichttp.New(context.Background(),
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTEL exporter: %w", err)
	}

	reader := sdkmetric.NewPeriodicReader(
		exporter,
		sdkmetric.WithInterval(30*time.Second),
	)

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
	)
	otel.SetMeterProvider(provider)

	meter := provider.Meter(serviceName)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &Exporter{
		reader:   reader,
		meter:    meter,
		memStats: memStats,
		lastGC:   memStats.NumGC,
	}, nil
}

func (e *Exporter) Close(ctx context.Context) error {
	if e.reader == nil {
		return nil
	}
	return e.reader.Shutdown(ctx)
}

func (e *Exporter) RecordCheckLatency(ctx context.Context, targetID string, latencyMs int64) {
	if e.meter == nil {
		return
	}
	counter, err := e.meter.Int64Counter("smoke_alarm.check.latency_ms")
	if err != nil {
		return
	}
	counter.Add(ctx, latencyMs, metric.WithAttributes(
		attribute.String("target_id", targetID),
	))
}

func (e *Exporter) RecordCheckFailure(ctx context.Context, targetID, failureClass string) {
	if e.meter == nil {
		return
	}
	counter, err := e.meter.Int64Counter("smoke_alarm.check.failures")
	if err != nil {
		return
	}
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("target_id", targetID),
		attribute.String("failure_class", failureClass),
	))
}

func (e *Exporter) RecordTargetState(ctx context.Context, targetID, state string) {
	if e.meter == nil {
		return
	}
	_, err := e.meter.Int64ObservableGauge("smoke_alarm.target.state",
		metric.WithDescription("Target state: 0=healthy, 1=degraded, 2=unhealthy, 3=outage, 4=regression"),
	)
	if err != nil {
		return
	}
}

func stateToInt(state string) int64 {
	switch state {
	case "healthy":
		return 0
	case "degraded":
		return 1
	case "unhealthy":
		return 2
	case "outage":
		return 3
	case "regression":
		return 4
	default:
		return -1
	}
}

func (e *Exporter) RecordSystemMetrics(ctx context.Context) {
	if e.meter == nil {
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memAlloc, _ := e.meter.Int64UpDownCounter("process.memory.alloc_bytes")
	memAlloc.Add(ctx, int64(memStats.Alloc), metric.WithAttributes(
		attribute.String("type", "current"),
	))

	memTotal, _ := e.meter.Int64UpDownCounter("process.memory.total_bytes")
	memTotal.Add(ctx, int64(memStats.TotalAlloc), metric.WithAttributes(
		attribute.String("type", "cumulative"),
	))

	goroutines, _ := e.meter.Int64UpDownCounter("process.goroutines")
	goroutines.Add(ctx, int64(runtime.NumGoroutine()), metric.WithAttributes(
		attribute.String("state", "active"),
	))

	gcCount, _ := e.meter.Int64UpDownCounter("process.gc.count")
	gcCount.Add(ctx, int64(memStats.NumGC-e.lastGC), metric.WithAttributes(
		attribute.String("type", "delta"),
	))
	e.lastGC = memStats.NumGC

	_ = memAlloc
	_ = memTotal
	_ = goroutines
	_ = gcCount
}
