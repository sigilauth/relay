package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/sigilauth/relay/internal/crypto"
	"github.com/sigilauth/relay/internal/handlers"
	appmiddleware "github.com/sigilauth/relay/internal/middleware"
	"github.com/sigilauth/relay/internal/push"
	"github.com/sigilauth/relay/internal/push/stdout"
	"github.com/sigilauth/relay/internal/ratelimit"
	"github.com/sigilauth/relay/internal/replay"
	"github.com/sigilauth/relay/internal/store"
	"github.com/sigilauth/relay/internal/store/memory"
	"github.com/sigilauth/relay/internal/telemetry"
	"github.com/sigilauth/relay/internal/verify"
	"github.com/sigilauth/relay/internal/websocket"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logLevel := getEnv("LOG_LEVEL", "info")
	lokiEndpoint := getEnv("LOKI_ENDPOINT", "")
	logger := telemetry.InitLogger(logLevel, lokiEndpoint)
	slog.SetDefault(logger)

	port := getEnv("PORT", "8080")
	databaseURL := getEnv("DATABASE_URL", "")
	serverPubKey := os.Getenv("SERVER_PUBLIC_KEY")
	otelEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	serviceName := getEnv("OTEL_SERVICE_NAME", "sigil-relay")
	mode := getEnv("RELAY_MODE", "production")

	if serverPubKey == "" {
		slog.Error("SERVER_PUBLIC_KEY environment variable is required")
		os.Exit(1)
	}

	verifier, err := verify.NewServerSignature(serverPubKey)
	if err != nil {
		slog.Error("Invalid SERVER_PUBLIC_KEY", "error", err)
		os.Exit(1)
	}

	serverPubKeyHex := hex.EncodeToString(verifier.RawBytes)
	serverPubKeyB64 := base64.StdEncoding.EncodeToString(verifier.RawBytes)
	slog.Info("Loaded server public key",
		"hex", serverPubKeyHex[:16]+"...",
		"b64", serverPubKeyB64[:16]+"...",
		"bytes", len(verifier.RawBytes))

	ctx := context.Background()

	shutdown, err := telemetry.InitTracer(ctx, serviceName, otelEndpoint)
	if err != nil {
		slog.Warn("Failed to initialize tracer", "error", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.Error("Failed to shutdown tracer", "error", err)
		}
	}()

	tokenEncryptionKey := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if tokenEncryptionKey == "" && mode == "production" {
		slog.Error("TOKEN_ENCRYPTION_KEY environment variable is required in production mode")
		os.Exit(1)
	}

	var encryptor *crypto.TokenEncryptor
	if tokenEncryptionKey != "" {
		encryptor, err = crypto.NewTokenEncryptor(tokenEncryptionKey)
		if err != nil {
			slog.Error("Failed to initialize token encryptor", "error", err)
			os.Exit(1)
		}
		slog.Info("Token encryption enabled")
	}

	var st store.Store
	var provider push.PushProvider

	switch mode {
	case "mock":
		slog.Info("Relay starting in MOCK mode (in-memory + stdout)")
		st = memory.New()
		provider = stdout.New("mock")
	case "production", "":
		if databaseURL != "" {
			if encryptor == nil {
				slog.Error("Token encryptor required for database mode")
				os.Exit(1)
			}
			pgStore, err := store.NewPgxStore(ctx, databaseURL, encryptor)
			if err != nil {
				slog.Error("Failed to connect to database", "error", err)
				os.Exit(1)
			}
			defer pgStore.Close()
			st = pgStore
			slog.Info("Connected to Postgres with encrypted token storage")
		} else {
			slog.Warn("No DATABASE_URL configured, using stub mode")
			st = &stubStore{}
		}
		provider = &mockProvider{}
	default:
		slog.Error("Unknown RELAY_MODE", "mode", mode)
		os.Exit(1)
	}

	// Initialize rate limiter (10 req/min per fingerprint)
	limiter := ratelimit.New(10.0/60.0, 1)

	// Initialize WebSocket manager (30s heartbeat interval)
	wsManager := websocket.NewManager(30 * time.Second)

	// Initialize challenge store for registration (5min TTL)
	challengeStore := store.NewChallengeStore()
	defer challengeStore.Close()

	// Initialize nonce store for push replay protection (120s TTL = 2x timestamp window)
	nonceStore := replay.NewNonceStore(120 * time.Second)
	defer nonceStore.Close()

	// Create HTTP router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(appmiddleware.StructuredLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(10 * time.Second))
	r.Use(appmiddleware.PrometheusMiddleware)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := fmt.Sprintf(`{"status":"healthy","version":"0.2.0","mode":"%s"}`, mode)
		w.Write([]byte(response))
	})

	// Metrics endpoint
	r.Get("/metrics", appmiddleware.MetricsHandler().ServeHTTP)

	// WebSocket endpoint
	r.Get("/ws", websocket.NewHandler(wsManager, st).ServeHTTP)

	// Relay endpoints
	r.Get("/devices/register/challenge", handlers.NewRegisterChallengeHandler(challengeStore).ServeHTTP)
	r.Post("/devices/register", handlers.NewRegisterHandler(st, challengeStore).ServeHTTP)
	r.Post("/push", handlers.NewPushHandler(st, provider, limiter, verifier, nonceStore).ServeHTTP)

	// Start HTTP server with OTel instrumentation
	addr := ":" + port
	srv := &http.Server{
		Addr:         addr,
		Handler:      otelhttp.NewHandler(r, "relay",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("Shutting down gracefully...")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Could not gracefully shutdown", "error", err)
			os.Exit(1)
		}
		close(done)
	}()

	slog.Info("Relay starting", "addr", addr, "otel_endpoint", otelEndpoint, "log_level", logLevel)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}

	<-done
	slog.Info("Server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// mockProvider is a temporary stub until APNs/FCM providers are implemented
type mockProvider struct{}

func (m *mockProvider) Send(ctx context.Context, token string, payload []byte) error {
	slog.Info("MOCK: Would send push", "token_prefix", token[:10], "payload", string(payload))
	return nil
}

func (m *mockProvider) Platform() string {
	return "mock"
}

// stubStore is a no-op store for testing without database
type stubStore struct{}

func (s *stubStore) RegisterDevice(ctx context.Context, fp, token, platform string) error {
	slog.Info("STUB: RegisterDevice", "fingerprint_prefix", fp[:16], "platform", platform)
	return nil
}
func (s *stubStore) GetPushToken(ctx context.Context, fp string) (*push.PushToken, error) {
	return nil, nil
}
func (s *stubStore) EvictToken(ctx context.Context, fp string) error        { return nil }
func (s *stubStore) IncrementFailures(ctx context.Context, fp string) error { return nil }
func (s *stubStore) ResetFailures(ctx context.Context, fp string) error     { return nil }
func (s *stubStore) GetStaleTokens(ctx context.Context, threshold, days int) ([]string, error) {
	return nil, nil
}
