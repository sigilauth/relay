package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		routePattern   string
		responseStatus int
		wantMethod     string
		wantRoute      string
		wantStatus     string
	}{
		{
			name:           "GET request with 200",
			method:         http.MethodGet,
			path:           "/health",
			routePattern:   "/health",
			responseStatus: http.StatusOK,
			wantMethod:     "GET",
			wantRoute:      "/health",
			wantStatus:     "200",
		},
		{
			name:           "POST request with 201",
			method:         http.MethodPost,
			path:           "/devices/register",
			routePattern:   "/devices/register",
			responseStatus: http.StatusCreated,
			wantMethod:     "POST",
			wantRoute:      "/devices/register",
			wantStatus:     "201",
		},
		{
			name:           "POST request with 400",
			method:         http.MethodPost,
			path:           "/push",
			routePattern:   "/push",
			responseStatus: http.StatusBadRequest,
			wantMethod:     "POST",
			wantRoute:      "/push",
			wantStatus:     "400",
		},
		{
			name:           "GET request with 404",
			method:         http.MethodGet,
			path:           "/unknown",
			routePattern:   "/unknown",
			responseStatus: http.StatusNotFound,
			wantMethod:     "GET",
			wantRoute:      "/unknown",
			wantStatus:     "404",
		},
		{
			name:           "POST request with 500",
			method:         http.MethodPost,
			path:           "/push",
			routePattern:   "/push",
			responseStatus: http.StatusInternalServerError,
			wantMethod:     "POST",
			wantRoute:      "/push",
			wantStatus:     "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpRequestDuration.Reset()

			r := chi.NewRouter()
			r.Use(PrometheusMiddleware)
			r.MethodFunc(tt.method, tt.routePattern, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
			})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			count := testutil.CollectAndCount(httpRequestDuration)
			if count == 0 {
				t.Error("Expected histogram to record samples, got 0")
			}

			metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			metricsW := httptest.NewRecorder()
			MetricsHandler().ServeHTTP(metricsW, metricsReq)

			metricsOutput := metricsW.Body.String()

			expectedCountMetric := `http_request_duration_seconds_count{method="` + tt.wantMethod + `",route="` + tt.wantRoute + `",status="` + tt.wantStatus + `"} 1`
			if !strings.Contains(metricsOutput, expectedCountMetric) {
				t.Errorf("Expected count metric not found:\n%s\nGot:\n%s", expectedCountMetric, metricsOutput)
			}

			expectedSumMetric := `http_request_duration_seconds_sum{method="` + tt.wantMethod + `",route="` + tt.wantRoute + `",status="` + tt.wantStatus + `"}`
			if !strings.Contains(metricsOutput, expectedSumMetric) {
				t.Errorf("Expected sum metric not found: %s", expectedSumMetric)
			}
		})
	}
}

func TestPrometheusMiddlewareBuckets(t *testing.T) {
	httpRequestDuration.Reset()

	r := chi.NewRouter()
	r.Use(PrometheusMiddleware)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	MetricsHandler().ServeHTTP(metricsW, metricsReq)

	metricsOutput := metricsW.Body.String()

	expectedBuckets := []string{
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="0.05"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="0.1"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="0.2"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="0.5"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="1"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="2"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="3"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="5"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="10"}`,
		`http_request_duration_seconds_bucket{method="GET",route="/health",status="200",le="+Inf"}`,
	}

	for _, bucket := range expectedBuckets {
		if !strings.Contains(metricsOutput, bucket) {
			t.Errorf("Expected bucket %q not found in metrics output", bucket)
		}
	}

	if !strings.Contains(metricsOutput, `http_request_duration_seconds_sum{method="GET",route="/health",status="200"}`) {
		t.Error("Expected _sum metric not found")
	}

	if !strings.Contains(metricsOutput, `http_request_duration_seconds_count{method="GET",route="/health",status="200"} 1`) {
		t.Error("Expected _count metric not found")
	}
}

func TestMetricsHandler(t *testing.T) {
	httpRequestDuration.Reset()

	handler := PrometheusMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()

	MetricsHandler().ServeHTTP(metricsW, metricsReq)

	if metricsW.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", metricsW.Code)
	}

	body := metricsW.Body.String()
	if !strings.Contains(body, "http_request_duration_seconds") {
		t.Error("Expected metrics output to contain http_request_duration_seconds")
	}
}
