package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

const instrumentationName = "github.com/radryc/memebank/server"

// Metrics holds the application-level instruments used by the API. All methods
// are safe to call on a nil *Metrics, which keeps instrumentation optional.
type Metrics struct {
	meter otelmetric.Meter

	memesCreated     otelmetric.Int64Counter
	templatesCreated otelmetric.Int64Counter
	votes            otelmetric.Int64Counter
	renders          otelmetric.Int64Counter
	renderDuration   otelmetric.Float64Histogram
}

// NewMetrics creates the memebank instruments using the global meter provider.
func NewMetrics() (*Metrics, error) {
	meter := otel.Meter(instrumentationName)

	memesCreated, err := meter.Int64Counter("memebank.memes.created",
		otelmetric.WithUnit("{meme}"),
		otelmetric.WithDescription("Number of published memes."),
	)
	if err != nil {
		return nil, err
	}
	templatesCreated, err := meter.Int64Counter("memebank.templates.created",
		otelmetric.WithUnit("{template}"),
		otelmetric.WithDescription("Number of templates added to the library."),
	)
	if err != nil {
		return nil, err
	}
	votes, err := meter.Int64Counter("memebank.votes",
		otelmetric.WithUnit("{vote}"),
		otelmetric.WithDescription("Number of vote changes."),
	)
	if err != nil {
		return nil, err
	}
	renders, err := meter.Int64Counter("memebank.renders",
		otelmetric.WithUnit("{render}"),
		otelmetric.WithDescription("Number of rendered meme images."),
	)
	if err != nil {
		return nil, err
	}
	renderDuration, err := meter.Float64Histogram("memebank.render.duration",
		otelmetric.WithUnit("s"),
		otelmetric.WithDescription("Time spent rendering a meme image."),
	)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		meter:            meter,
		memesCreated:     memesCreated,
		templatesCreated: templatesCreated,
		votes:            votes,
		renders:          renders,
		renderDuration:   renderDuration,
	}, nil
}

// MemeCreated records a published meme.
func (m *Metrics) MemeCreated(ctx context.Context, animated bool) {
	if m == nil {
		return
	}
	m.memesCreated.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.Bool("memebank.meme.animated", animated),
	))
}

// TemplateCreated records a new template.
func (m *Metrics) TemplateCreated(ctx context.Context, source string) {
	if m == nil {
		return
	}
	m.templatesCreated.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("memebank.template.source", source),
	))
}

// Vote records a vote change. A resulting vote of 0 means the vote was toggled
// off.
func (m *Metrics) Vote(ctx context.Context, value int) {
	if m == nil {
		return
	}
	direction := "clear"
	switch value {
	case 1:
		direction = "up"
	case -1:
		direction = "down"
	}
	m.votes.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("memebank.vote.direction", direction),
	))
}

// Render records one rendered image and how long it took.
func (m *Metrics) Render(ctx context.Context, animated bool, duration time.Duration) {
	if m == nil {
		return
	}
	attrs := otelmetric.WithAttributes(
		attribute.Bool("memebank.render.animated", animated),
	)
	m.renders.Add(ctx, 1, attrs)
	m.renderDuration.Record(ctx, duration.Seconds(), attrs)
}

// RegisterCacheSize reports the current rendered image cache size as a gauge.
func (m *Metrics) RegisterCacheSize(size func() int64) error {
	if m == nil {
		return nil
	}
	_, err := m.meter.Int64ObservableGauge("memebank.image_cache.bytes",
		otelmetric.WithUnit("By"),
		otelmetric.WithDescription("Bytes currently held by the rendered image cache."),
		otelmetric.WithInt64Callback(func(_ context.Context, observer otelmetric.Int64Observer) error {
			observer.Observe(size())
			return nil
		}),
	)
	return err
}
