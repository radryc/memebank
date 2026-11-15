package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServiceName(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "")
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
		if got := serviceName(); got != "memebank" {
			t.Fatalf("serviceName() = %q, want memebank", got)
		}
	})

	t.Run("OTEL_SERVICE_NAME wins", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "memebank.api")
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.name=other")
		if got := serviceName(); got != "memebank.api" {
			t.Fatalf("serviceName() = %q, want memebank.api", got)
		}
	})

	t.Run("resource attributes", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "")
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.namespace=memebank,service.name=memebank.app.memebank")
		if got := serviceName(); got != "memebank.app.memebank" {
			t.Fatalf("serviceName() = %q, want memebank.app.memebank", got)
		}
	})

	t.Run("encoded resource attribute", func(t *testing.T) {
		t.Setenv("OTEL_SERVICE_NAME", "")
		t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "service.name=memebank%20app")
		if got := serviceName(); got != "memebank app" {
			t.Fatalf("serviceName() = %q, want %q", got, "memebank app")
		}
	})
}

func TestSamplerFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		sampler string
		arg     string
		want    string
	}{
		{name: "default", want: "ParentBased"},
		{name: "parentbased always on", sampler: "parentbased_always_on", want: "ParentBased"},
		{name: "always off", sampler: "always_off", want: "AlwaysOffSampler"},
		{name: "ratio", sampler: "traceidratio", arg: "0.25", want: "0.25"},
		{name: "unsupported", sampler: "jaeger_remote", want: "ParentBased"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OTEL_TRACES_SAMPLER", tt.sampler)
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tt.arg)
			got := samplerFromEnv(testLogger()).Description()
			if !strings.Contains(got, tt.want) {
				t.Fatalf("sampler description = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestPropagatorFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("OTEL_PROPAGATORS", "")
		fields := propagatorFromEnv(testLogger()).Fields()
		for _, want := range []string{"traceparent", "tracestate", "baggage"} {
			if !contains(fields, want) {
				t.Fatalf("propagator fields = %v, missing %q", fields, want)
			}
		}
	})

	t.Run("none", func(t *testing.T) {
		t.Setenv("OTEL_PROPAGATORS", "none")
		if fields := propagatorFromEnv(testLogger()).Fields(); len(fields) != 0 {
			t.Fatalf("propagator fields = %v, want none", fields)
		}
	})

	t.Run("unsupported falls back", func(t *testing.T) {
		t.Setenv("OTEL_PROPAGATORS", "b3")
		fields := propagatorFromEnv(testLogger()).Fields()
		if !contains(fields, "traceparent") {
			t.Fatalf("propagator fields = %v, want tracecontext fallback", fields)
		}
	})
}

func TestEndpointFor(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
		if got := endpointFor("traces"); got != "http://collector:4318" {
			t.Fatalf("endpointFor(traces) = %q", got)
		}
	})

	t.Run("signal override", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://traces:4318")
		if got := endpointFor("traces"); got != "http://traces:4318" {
			t.Fatalf("endpointFor(traces) = %q", got)
		}
	})

	t.Run("logs override", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "http://logs:4318")
		if got := endpointFor("logs"); got != "http://logs:4318" {
			t.Fatalf("endpointFor(logs) = %q", got)
		}
	})
}

func TestExporterEnabled(t *testing.T) {
	log := testLogger()

	t.Run("disabled without endpoint", func(t *testing.T) {
		t.Setenv("OTEL_SDK_DISABLED", "")
		t.Setenv("OTEL_TRACES_EXPORTER", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
		if exporterEnabled("traces", log) {
			t.Fatal("exporterEnabled(traces) = true, want false without endpoint")
		}
	})

	t.Run("enabled with endpoint", func(t *testing.T) {
		t.Setenv("OTEL_SDK_DISABLED", "")
		t.Setenv("OTEL_TRACES_EXPORTER", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		if !exporterEnabled("traces", log) {
			t.Fatal("exporterEnabled(traces) = false, want true")
		}
	})

	t.Run("none exporter", func(t *testing.T) {
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_TRACES_EXPORTER", "none")
		if exporterEnabled("traces", log) {
			t.Fatal("exporterEnabled(traces) = true, want false for none")
		}
	})

	t.Run("sdk disabled", func(t *testing.T) {
		t.Setenv("OTEL_SDK_DISABLED", "true")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_TRACES_EXPORTER", "")
		if exporterEnabled("traces", log) {
			t.Fatal("exporterEnabled(traces) = true, want false when disabled")
		}
	})

	t.Run("logs enabled with endpoint", func(t *testing.T) {
		t.Setenv("OTEL_SDK_DISABLED", "")
		t.Setenv("OTEL_LOGS_EXPORTER", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
		if !exporterEnabled("logs", log) {
			t.Fatal("exporterEnabled(logs) = false, want true")
		}
	})

	t.Run("logs none exporter", func(t *testing.T) {
		t.Setenv("OTEL_SDK_DISABLED", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4318")
		t.Setenv("OTEL_LOGS_EXPORTER", "none")
		if exporterEnabled("logs", log) {
			t.Fatal("exporterEnabled(logs) = true, want false for none")
		}
	})
}

func TestSetupFallsBackToStdout(t *testing.T) {
	previous := slog.Default()
	slog.SetDefault(testLogger())
	t.Cleanup(func() { slog.SetDefault(previous) })

	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	t.Setenv("OTEL_TRACES_EXPORTER", "")
	t.Setenv("OTEL_METRICS_EXPORTER", "")
	t.Setenv("OTEL_LOGS_EXPORTER", "")

	shutdown, handler, err := Setup(context.Background(), "test", slog.LevelInfo)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	}()

	wrapped, ok := handler.(logHandler)
	if !ok {
		t.Fatalf("handler = %T, want logHandler wrapper", handler)
	}
	if _, ok := wrapped.Handler.(*slog.TextHandler); !ok {
		t.Fatalf("fallback handler = %T, want *slog.TextHandler", wrapped.Handler)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
