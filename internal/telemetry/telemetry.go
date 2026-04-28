package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func InitLogger(level string, lokiEndpoint string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	stdoutHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})

	if lokiEndpoint == "" {
		return slog.New(stdoutHandler)
	}

	hostname, _ := os.Hostname()
	env := getEnv("ENV", "prod")

	lokiHandler := NewLokiHandler(lokiEndpoint, map[string]string{
		"service": "sigil-relay",
		"env":     env,
		"host":    hostname,
	}, logLevel)

	multiHandler := NewMultiHandler(stdoutHandler, lokiHandler)
	return slog.New(multiHandler)
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func InitTracer(ctx context.Context, serviceName, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		slog.Warn("OTEL_EXPORTER_OTLP_ENDPOINT not set, tracing disabled")
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		slog.Warn("Failed to create OTel resource", "error", err)
		return func(context.Context) error { return nil }, nil
	}

	// Strip http:// or https:// prefix if present (gRPC expects host:port only)
	cleanEndpoint := strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cleanEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		slog.Warn("Failed to create OTel exporter, continuing without tracing", "error", err, "endpoint", endpoint)
		return func(context.Context) error { return nil }, nil
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("OTel tracer initialized", "endpoint", endpoint, "service", serviceName)

	return tp.Shutdown, nil
}

// MultiHandler wraps two slog.Handlers and calls both.
type MultiHandler struct {
	handlers []slog.Handler
}

func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (m *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, h := range m.handlers {
		if h.Enabled(ctx, record.Level) {
			_ = h.Handle(ctx, record)
		}
	}
	return nil
}

func (m *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

func (m *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}

// LokiHandler batches log entries and POSTs to Loki push API.
type LokiHandler struct {
	endpoint string
	labels   map[string]string
	level    slog.Level
	mu       sync.Mutex
	buffer   []lokiEntry
	client   *http.Client
	ticker   *time.Ticker
	done     chan struct{}
}

type lokiEntry struct {
	timestamp int64
	line      string
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

func NewLokiHandler(endpoint string, labels map[string]string, level slog.Level) *LokiHandler {
	h := &LokiHandler{
		endpoint: endpoint,
		labels:   labels,
		level:    level,
		buffer:   make([]lokiEntry, 0, 100),
		client:   &http.Client{Timeout: 3 * time.Second},
		ticker:   time.NewTicker(5 * time.Second),
		done:     make(chan struct{}),
	}

	go h.flushLoop()
	return h
}

func (h *LokiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *LokiHandler) Handle(_ context.Context, record slog.Record) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)

	logMap := map[string]interface{}{
		"time":  record.Time.Format(time.RFC3339Nano),
		"level": record.Level.String(),
		"msg":   record.Message,
	}

	record.Attrs(func(attr slog.Attr) bool {
		logMap[attr.Key] = attr.Value.Any()
		return true
	})

	if err := enc.Encode(logMap); err != nil {
		return err
	}

	entry := lokiEntry{
		timestamp: record.Time.UnixNano(),
		line:      strings.TrimSpace(buf.String()),
	}

	h.mu.Lock()
	h.buffer = append(h.buffer, entry)
	shouldFlush := len(h.buffer) >= 100
	h.mu.Unlock()

	if shouldFlush {
		go h.flush()
	}

	return nil
}

func (h *LokiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *LokiHandler) WithGroup(name string) slog.Handler {
	return h
}

func (h *LokiHandler) flushLoop() {
	for {
		select {
		case <-h.ticker.C:
			h.flush()
		case <-h.done:
			h.ticker.Stop()
			return
		}
	}
}

func (h *LokiHandler) flush() {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}

	entries := make([]lokiEntry, len(h.buffer))
	copy(entries, h.buffer)
	h.buffer = h.buffer[:0]
	h.mu.Unlock()

	values := make([][]string, len(entries))
	for i, entry := range entries {
		values[i] = []string{
			fmt.Sprintf("%d", entry.timestamp),
			entry.line,
		}
	}

	req := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: h.labels,
				Values: values,
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		slog.Warn("Failed to marshal Loki request", "error", err)
		return
	}

	httpReq, err := http.NewRequest("POST", h.endpoint+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		slog.Warn("Failed to create Loki request", "error", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Warn("Loki push failed", "status", resp.StatusCode)
	}
}

func (h *LokiHandler) Close() {
	close(h.done)
	h.flush()
}
