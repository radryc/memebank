// Package observability wires OpenTelemetry tracing, metrics and logs into
// memebank using the standard OTEL_* environment variables.
//
// All signals are exported over OTLP/HTTP. Exporting is only enabled when an
// OTLP endpoint is configured, so local development stays on stdout unless
// OTEL_EXPORTER_OTLP_ENDPOINT (or a signal-specific endpoint) is set.
package observability

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// Setup configures the global OpenTelemetry tracer, meter and logger providers
// from the OTEL_* environment. It returns a shutdown function that flushes
// pending telemetry, plus the slog handler the application should log through.
//
// When logs are exported over OTLP the handler bridges slog into the OTel logs
// SDK. Otherwise it falls back to text on stdout, which keeps local development
// readable without a collector. The shutdown function is always safe to call,
// even when exporting is disabled.
func Setup(ctx context.Context, version string, level slog.Leveler) (func(context.Context) error, slog.Handler, error) {
	log := slog.Default()

	if disabled() {
		log.Info("observability disabled by OTEL_SDK_DISABLED")
		return noopShutdown, stdoutLogHandler(level), nil
	}

	otel.SetTextMapPropagator(propagatorFromEnv(log))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Warn("opentelemetry error", "error", err)
	}))

	res, err := buildResource(ctx, version)
	if err != nil {
		return nil, nil, fmt.Errorf("build otel resource: %w", err)
	}

	var shutdowns []func(context.Context) error
	var handler slog.Handler
	var logShutdown func(context.Context) error

	// Logs are configured first: once the caller installs handler, the setup
	// messages below keep using the bootstrap logger on stdout/stderr.
	if exporterEnabled("logs", log) {
		exp, err := otlploghttp.New(ctx)
		if err != nil {
			log.Error("could not create OTLP log exporter", "error", err)
		} else {
			lp := sdklog.NewLoggerProvider(
				sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
				sdklog.WithResource(res),
			)
			otel.SetLoggerProvider(lp)
			// Shut the log provider down last so shutdown errors from the
			// other signals can still be recorded.
			logShutdown = lp.Shutdown
			handler = otelslog.NewHandler(instrumentationName,
				otelslog.WithLoggerProvider(lp),
				otelslog.WithVersion(version),
			)
			log.Info("otlp logs enabled", "endpoint", endpointFor("logs"))
		}
	}

	if exporterEnabled("traces", log) {
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			log.Error("could not create OTLP trace exporter", "error", err)
		} else {
			tp := sdktrace.NewTracerProvider(
				sdktrace.WithBatcher(exp),
				sdktrace.WithResource(res),
				sdktrace.WithSampler(samplerFromEnv(log)),
			)
			otel.SetTracerProvider(tp)
			shutdowns = append(shutdowns, tp.Shutdown)
			log.Info("otlp traces enabled", "endpoint", endpointFor("traces"))
		}
	}

	if exporterEnabled("metrics", log) {
		exp, err := otlpmetrichttp.New(ctx)
		if err != nil {
			log.Error("could not create OTLP metric exporter", "error", err)
		} else {
			mp := metric.NewMeterProvider(
				metric.WithReader(metric.NewPeriodicReader(exp)),
				metric.WithResource(res),
			)
			otel.SetMeterProvider(mp)
			shutdowns = append(shutdowns, mp.Shutdown)
			log.Info("otlp metrics enabled", "endpoint", endpointFor("metrics"))
		}
	}

	if logShutdown != nil {
		shutdowns = append(shutdowns, logShutdown)
	}

	if handler == nil {
		handler = stdoutLogHandler(level)
	}

	if len(shutdowns) == 0 {
		if endpointFor("traces") == "" && endpointFor("metrics") == "" && endpointFor("logs") == "" {
			log.Info("otel exporters disabled; set OTEL_EXPORTER_OTLP_ENDPOINT to export traces, metrics and logs")
		}
		return noopShutdown, handler, nil
	}

	return func(ctx context.Context) error {
		var errs []error
		for _, shutdown := range shutdowns {
			errs = append(errs, shutdown(ctx))
		}
		return errors.Join(errs...)
	}, handler, nil
}

// stdoutLogHandler keeps development output readable when OTLP logs are not
// configured. Trace and span IDs are added manually because there is no OTel
// log record to carry them.
func stdoutLogHandler(level slog.Leveler) slog.Handler {
	return NewLogHandler(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// HTTPHandler wraps h with OTel server spans and HTTP metrics. Static assets and
// uploaded images are not traced; only API requests are.
func HTTPHandler(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, "memebank",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return strings.HasPrefix(r.URL.Path, "/api/")
		}),
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			if r.Pattern != "" {
				return r.Method + " " + r.Pattern
			}
			return r.Method + " /api"
		}),
	)
}

// InstrumentDB registers database/sql pool metrics for db. The returned cleanup
// function unregisters the metrics.
func InstrumentDB(db *sql.DB) (func(), error) {
	reg, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(semconv.DBSystemNameSQLite),
	)
	if err != nil {
		return nil, err
	}
	return func() { _ = reg.Unregister() }, nil
}

// NewLogHandler decorates next so log records with a span in their context
// include trace_id and span_id attributes. This correlates logs with traces.
func NewLogHandler(next slog.Handler) slog.Handler {
	return logHandler{Handler: next}
}

type logHandler struct {
	slog.Handler
}

func (h logHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
}

func (h logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return logHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h logHandler) WithGroup(name string) slog.Handler {
	return logHandler{Handler: h.Handler.WithGroup(name)}
}

func noopShutdown(context.Context) error { return nil }

func disabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true")
}

func endpointFor(signal string) string {
	var specific string
	switch signal {
	case "traces":
		specific = os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	case "metrics":
		specific = os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT")
	case "logs":
		specific = os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT")
	}
	if strings.TrimSpace(specific) != "" {
		return specific
	}
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
}

// exporterEnabled reports whether signal should be exported. Only the OTLP
// exporter is supported; the default localhost endpoint is not used implicitly
// so an unconfigured process does not retry against a missing collector.
func exporterEnabled(signal string, log *slog.Logger) bool {
	if disabled() {
		return false
	}
	var name string
	switch signal {
	case "traces":
		name = os.Getenv("OTEL_TRACES_EXPORTER")
	case "metrics":
		name = os.Getenv("OTEL_METRICS_EXPORTER")
	case "logs":
		name = os.Getenv("OTEL_LOGS_EXPORTER")
	}
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "otlp":
	case "none":
		return false
	default:
		log.Warn("unsupported otel exporter; using none", "signal", signal, "exporter", name)
		return false
	}
	if endpointFor(signal) == "" {
		log.Debug("otel endpoint not set; exporter disabled", "signal", signal)
		return false
	}
	return true
}

func buildResource(ctx context.Context, version string) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{semconv.ServiceName(serviceName())}
	if version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	extra, err := resource.New(ctx, resource.WithAttributes(attrs...))
	if err != nil {
		return nil, err
	}
	return resource.Merge(resource.Default(), extra)
}

// serviceName honours OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES, falling
// back to the application name instead of the SDK's unknown_service default.
func serviceName() string {
	if name := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME")); name != "" {
		return name
	}
	for _, pair := range strings.Split(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"), ",") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) != "service.name" {
			continue
		}
		if decoded, err := url.QueryUnescape(strings.TrimSpace(value)); err == nil {
			value = decoded
		}
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "memebank"
}

func samplerFromEnv(log *slog.Logger) sdktrace.Sampler {
	ratio := samplerRatio(log)
	switch strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER"))) {
	case "always_on":
		return sdktrace.AlwaysSample()
	case "always_off":
		return sdktrace.NeverSample()
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(ratio)
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	default:
		log.Warn("unsupported OTEL_TRACES_SAMPLER; using parentbased_always_on",
			"sampler", os.Getenv("OTEL_TRACES_SAMPLER"))
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

func samplerRatio(log *slog.Logger) float64 {
	raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	if raw == "" {
		return 1
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil || ratio < 0 || ratio > 1 {
		log.Warn("invalid OTEL_TRACES_SAMPLER_ARG; using 1.0", "value", raw)
		return 1
	}
	return ratio
}

func propagatorFromEnv(log *slog.Logger) propagation.TextMapPropagator {
	raw := strings.TrimSpace(os.Getenv("OTEL_PROPAGATORS"))
	if raw == "" {
		raw = "tracecontext,baggage"
	}
	var props []propagation.TextMapPropagator
	for _, entry := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(entry)) {
		case "tracecontext":
			props = append(props, propagation.TraceContext{})
		case "baggage":
			props = append(props, propagation.Baggage{})
		case "none":
			return propagation.NewCompositeTextMapPropagator()
		case "":
		default:
			log.Warn("unsupported OTEL_PROPAGATORS entry; ignoring", "propagator", entry)
		}
	}
	if len(props) == 0 {
		log.Warn("no supported OTEL_PROPAGATORS; using tracecontext,baggage")
		return propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	}
	return propagation.NewCompositeTextMapPropagator(props...)
}
