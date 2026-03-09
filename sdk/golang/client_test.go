// SPDX-License-Identifier: MIT

package golang

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kolapsis/shm/pkg/crypto"
)

// =============================================================================
// SLUG FUNCTION TESTS
// =============================================================================

func TestSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"My App", "my-app"},
		{"MyApp", "myapp"},
		{"my_app", "my-app"},
		{"My-App", "my-app"},
		{"  spaced  ", "spaced"},
		{"UPPERCASE", "uppercase"},
		{"été", "ete"},
		{"café", "cafe"},
		{"niño", "nino"},
		{"", "app"},
		{"---", "app"},
		{"123app", "123app"},
		{"app123", "app123"},
		{"app@#$%name", "appname"},
		{"Très Spécial Àpp", "tres-special-app"},
		{"multiple   spaces", "multiple-spaces"},
		{"a--b", "a-b"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := slug(tt.input)
			if result != tt.expected {
				t.Errorf("slug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// DEPLOYMENT MODE DETECTION TESTS
// =============================================================================

func TestDetectDeploymentMode_Standalone(t *testing.T) {
	// Clear kubernetes env vars
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_PORT")

	// In a normal test environment without docker, should return standalone
	mode := detectDeploymentMode()

	// Can be standalone or docker depending on test environment
	if mode != "standalone" && mode != "docker" {
		t.Errorf("detectDeploymentMode() = %q, want 'standalone' or 'docker'", mode)
	}
}

func TestDetectDeploymentMode_Kubernetes(t *testing.T) {
	// Set kubernetes env vars
	os.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	os.Setenv("KUBERNETES_PORT", "443")
	defer func() {
		os.Unsetenv("KUBERNETES_SERVICE_HOST")
		os.Unsetenv("KUBERNETES_PORT")
	}()

	mode := detectDeploymentMode()
	if mode != "kubernetes" {
		t.Errorf("detectDeploymentMode() = %q, want 'kubernetes'", mode)
	}
}

// =============================================================================
// BYTES TO MB TESTS
// =============================================================================

func TestBytesToMB(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected uint64
	}{
		{0, 0},
		{1024, 0},                  // Less than 1 MB
		{1024 * 1024, 1},           // Exactly 1 MB
		{2 * 1024 * 1024, 2},       // 2 MB
		{1024 * 1024 * 1024, 1024}, // 1 GB = 1024 MB
	}

	for _, tt := range tests {
		result := bytesToMB(tt.bytes)
		if result != tt.expected {
			t.Errorf("bytesToMB(%d) = %d, want %d", tt.bytes, result, tt.expected)
		}
	}
}

// =============================================================================
// IDENTITY TESTS
// =============================================================================

func TestLoadOrGenerateIdentity_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "test_identity.json")

	id, err := loadOrGenerateIdentity(idPath)
	if err != nil {
		t.Fatalf("loadOrGenerateIdentity() error = %v", err)
	}

	if id.InstanceID == "" {
		t.Error("InstanceID should not be empty")
	}
	if id.PublicKey == "" {
		t.Error("PublicKey should not be empty")
	}
	if id.PrivateKey == "" {
		t.Error("PrivateKey should not be empty")
	}

	// Verify file was created
	if _, err := os.Stat(idPath); os.IsNotExist(err) {
		t.Error("identity file should have been created")
	}

	// Verify keys are valid hex
	_, err = hex.DecodeString(id.PublicKey)
	if err != nil {
		t.Errorf("PublicKey is not valid hex: %v", err)
	}
	_, err = hex.DecodeString(id.PrivateKey)
	if err != nil {
		t.Errorf("PrivateKey is not valid hex: %v", err)
	}
}

func TestLoadOrGenerateIdentity_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "test_identity.json")

	// Generate first identity
	id1, _ := loadOrGenerateIdentity(idPath)

	// Load again - should return same identity
	id2, err := loadOrGenerateIdentity(idPath)
	if err != nil {
		t.Fatalf("second loadOrGenerateIdentity() error = %v", err)
	}

	if id1.InstanceID != id2.InstanceID {
		t.Error("InstanceID should be the same when loading existing file")
	}
	if id1.PublicKey != id2.PublicKey {
		t.Error("PublicKey should be the same when loading existing file")
	}
	if id1.PrivateKey != id2.PrivateKey {
		t.Error("PrivateKey should be the same when loading existing file")
	}
}

func TestLoadOrGenerateIdentity_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "corrupted_identity.json")

	// Write corrupted JSON
	os.WriteFile(idPath, []byte("not valid json {{{"), 0600)

	// Should regenerate new identity
	id, err := loadOrGenerateIdentity(idPath)
	if err != nil {
		t.Fatalf("should handle corrupted file: %v", err)
	}

	if id.InstanceID == "" {
		t.Error("should have generated new identity")
	}
}

func TestIdentity_SignatureWorks(t *testing.T) {
	tmpDir := t.TempDir()
	idPath := filepath.Join(tmpDir, "test_identity.json")

	id, _ := loadOrGenerateIdentity(idPath)

	// Sign a message using the identity
	message := []byte("test message")
	privBytes, _ := hex.DecodeString(id.PrivateKey)
	signature := crypto.Sign(privBytes, message)

	// Verify using the public key
	if !crypto.Verify(id.PublicKey, message, signature) {
		t.Error("signature created with identity should verify")
	}
}

// =============================================================================
// CLIENT CONFIG TESTS
// =============================================================================

func TestNew_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ServerURL:  "http://localhost:8080",
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Default ReportInterval should be 1 hour
	if client.config.ReportInterval != 1*time.Hour {
		t.Errorf("default ReportInterval = %v, want 1h", client.config.ReportInterval)
	}
}

func TestNew_MinimumReportInterval(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ServerURL:      "http://localhost:8080",
		AppName:        "test-app",
		AppVersion:     "1.0.0",
		DataDir:        tmpDir,
		ReportInterval: 10 * time.Second, // Too short!
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Should be bumped to minimum of 1 minute
	if client.config.ReportInterval != time.Minute {
		t.Errorf("ReportInterval = %v, want minimum 1m", client.config.ReportInterval)
	}
}

func TestNew_EmptyDataDir(t *testing.T) {
	// Change to temp dir to avoid polluting current dir
	originalDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(originalDir)

	cfg := Config{
		ServerURL:  "http://localhost:8080",
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    "", // Empty should default to "."
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if client.config.DataDir != "." {
		t.Errorf("DataDir = %q, want '.'", client.config.DataDir)
	}
}

// =============================================================================
// CLIENT DISABLED TELEMETRY TESTS
// =============================================================================

func TestClient_DisabledTelemetry(t *testing.T) {
	tmpDir := t.TempDir()

	// Server that should NOT receive any requests
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:  server.URL,
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    false, // Disabled!
	}

	client, _ := New(cfg)

	// Start with context that cancels immediately
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)

	if requestCount > 0 {
		t.Errorf("disabled client made %d requests, should make 0", requestCount)
	}
}

// =============================================================================
// CLIENT HTTP INTERACTION TESTS
// =============================================================================

func TestClient_RegisterRequest(t *testing.T) {
	tmpDir := t.TempDir()

	var receivedReq map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/register" {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedReq)
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:   server.URL,
		AppName:     "test-app",
		AppVersion:  "1.0.0",
		DataDir:     tmpDir,
		Environment: "test",
		Enabled:     true,
	}

	client, _ := New(cfg)
	err := client.register()

	if err != nil {
		t.Fatalf("register() error = %v", err)
	}

	if receivedReq["app_name"] != "test-app" {
		t.Errorf("app_name = %v, want 'test-app'", receivedReq["app_name"])
	}
	if receivedReq["app_version"] != "1.0.0" {
		t.Errorf("app_version = %v, want '1.0.0'", receivedReq["app_version"])
	}
	if receivedReq["instance_id"] == "" {
		t.Error("instance_id should not be empty")
	}
	if receivedReq["public_key"] == "" {
		t.Error("public_key should not be empty")
	}
}

func TestClient_ActivateRequest_Signed(t *testing.T) {
	tmpDir := t.TempDir()

	var signature string
	var instanceID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/activate" {
			signature = r.Header.Get("X-Signature")
			instanceID = r.Header.Get("X-Instance-ID")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:  server.URL,
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    true,
	}

	client, _ := New(cfg)
	client.activate()

	if signature == "" {
		t.Error("X-Signature header should be present")
	}
	if instanceID == "" {
		t.Error("X-Instance-ID header should be present")
	}

	// Verify signature is valid hex
	_, err := hex.DecodeString(signature)
	if err != nil {
		t.Errorf("signature is not valid hex: %v", err)
	}
}

func TestClient_SnapshotRequest_Signed(t *testing.T) {
	tmpDir := t.TempDir()

	var signature string
	var body map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/snapshot" {
			signature = r.Header.Get("X-Signature")
			bodyBytes, _ := io.ReadAll(r.Body)
			json.Unmarshal(bodyBytes, &body)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:  server.URL,
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    true,
	}

	client, _ := New(cfg)
	client.SetProvider(func() map[string]interface{} {
		return map[string]interface{}{
			"custom_metric": 42,
		}
	})
	client.sendSnapshot()

	if signature == "" {
		t.Error("X-Signature header should be present on snapshot")
	}

	// Verify metrics contain custom metric
	if metrics, ok := body["metrics"].(map[string]interface{}); ok {
		if metrics["custom_metric"] != float64(42) {
			t.Errorf("custom_metric = %v, want 42", metrics["custom_metric"])
		}
	}
}

func TestClient_GetSystemMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ServerURL:  "http://localhost:8080",
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
	}

	client, _ := New(cfg)
	client.startTime = time.Now().Add(-2 * time.Hour)

	metrics := client.getSystemMetrics()

	// Check required fields exist
	requiredFields := []string{
		"sys_os", "sys_arch", "sys_cpu_cores", "sys_go_version",
		"sys_mode", "app_mem_alloc_mb", "app_goroutines",
	}

	for _, field := range requiredFields {
		if _, ok := metrics[field]; !ok {
			t.Errorf("missing system metric: %s", field)
		}
	}

	// Check uptime is calculated
	if uptime, ok := metrics["app_uptime_h"].(int); ok {
		if uptime < 1 {
			t.Errorf("app_uptime_h = %d, expected >= 1", uptime)
		}
	}
}

// =============================================================================
// ENSURE DATA DIR TESTS
// =============================================================================

func TestEnsureDataDir_CreatesNestedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "a", "b", "c")

	ensureDataDir(nestedDir)

	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("ensureDataDir should create nested directories")
	}
}

func TestEnsureDataDir_ExistingDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Should not panic or error on existing dir
	ensureDataDir(tmpDir)
	ensureDataDir(tmpDir) // Call twice
}

// =============================================================================
// IS CONTAINER CGROUP TESTS
// =============================================================================

func TestIsContainerCgroup(t *testing.T) {
	// This test just verifies the function doesn't panic
	// Actual result depends on test environment
	result := isContainerCgroup()
	_ = result // Can be true or false depending on environment
}

// =============================================================================
// CLIENT PROVIDER TESTS
// =============================================================================

func TestClient_SetProvider(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ServerURL:  "http://localhost:8080",
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
	}

	client, _ := New(cfg)

	called := false
	client.SetProvider(func() map[string]interface{} {
		called = true
		return map[string]interface{}{"test": 1}
	})

	if client.provider == nil {
		t.Error("provider should be set")
	}

	// Trigger provider call
	client.provider()
	if !called {
		t.Error("provider function should have been called")
	}
}

// =============================================================================
// ERROR HANDLING TESTS
// =============================================================================

func TestClient_RegisterError_ServerDown(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		ServerURL:  "http://localhost:99999", // Invalid port
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    true,
	}

	client, _ := New(cfg)
	err := client.register()

	if err == nil {
		t.Error("register() should return error when server is down")
	}
}

func TestClient_RegisterError_BadStatusCode(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:  server.URL,
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    true,
	}

	client, _ := New(cfg)
	err := client.register()

	if err == nil {
		t.Error("register() should return error on 500 status")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code: %v", err)
	}
}

// =============================================================================
// COLLECT SYSTEM METRICS TESTS
// =============================================================================

func TestClient_SnapshotWithSystemMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	var receivedMetrics map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/snapshot" {
			bodyBytes, _ := io.ReadAll(r.Body)
			var body map[string]interface{}
			json.Unmarshal(bodyBytes, &body)
			if metricsRaw, ok := body["metrics"].(string); ok {
				json.Unmarshal([]byte(metricsRaw), &receivedMetrics)
			} else if metricsMap, ok := body["metrics"].(map[string]interface{}); ok {
				receivedMetrics = metricsMap
			}
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:            server.URL,
		AppName:              "test-app",
		AppVersion:           "1.0.0",
		DataDir:              tmpDir,
		Enabled:              true,
		CollectSystemMetrics: true,
	}

	client, _ := New(cfg)
	client.SetProvider(func() map[string]interface{} {
		return map[string]interface{}{"custom_metric": 123}
	})
	client.sendSnapshot()

	// Should contain system metrics
	systemFields := []string{"sys_os", "sys_arch", "sys_cpu_cores", "sys_go_version", "sys_mode"}
	for _, field := range systemFields {
		if _, ok := receivedMetrics[field]; !ok {
			t.Errorf("expected system metric %q when CollectSystemMetrics=true", field)
		}
	}

	// Should also contain custom metric
	if receivedMetrics["custom_metric"] != float64(123) {
		t.Errorf("custom_metric = %v, want 123", receivedMetrics["custom_metric"])
	}
}

func TestClient_SnapshotWithoutSystemMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	var receivedMetrics map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/snapshot" {
			bodyBytes, _ := io.ReadAll(r.Body)
			var body map[string]interface{}
			json.Unmarshal(bodyBytes, &body)
			if metricsRaw, ok := body["metrics"].(string); ok {
				json.Unmarshal([]byte(metricsRaw), &receivedMetrics)
			} else if metricsMap, ok := body["metrics"].(map[string]interface{}); ok {
				receivedMetrics = metricsMap
			}
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := Config{
		ServerURL:            server.URL,
		AppName:              "test-app",
		AppVersion:           "1.0.0",
		DataDir:              tmpDir,
		Enabled:              true,
		CollectSystemMetrics: false,
	}

	client, _ := New(cfg)
	client.SetProvider(func() map[string]interface{} {
		return map[string]interface{}{"custom_metric": 456}
	})
	client.sendSnapshot()

	// Should NOT contain system metrics
	systemFields := []string{"sys_os", "sys_arch", "sys_cpu_cores", "sys_go_version", "sys_mode"}
	for _, field := range systemFields {
		if _, ok := receivedMetrics[field]; ok {
			t.Errorf("unexpected system metric %q when CollectSystemMetrics=false", field)
		}
	}

	// Should still contain custom metric
	if receivedMetrics["custom_metric"] != float64(456) {
		t.Errorf("custom_metric = %v, want 456", receivedMetrics["custom_metric"])
	}
}

func TestCollectSystemMetricsFromEnv(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"", true}, // absent = enabled
		{"true", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"anything", true}, // unknown = enabled
	}

	for _, tt := range tests {
		t.Run("env="+tt.envValue, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("SHM_COLLECT_SYSTEM_METRICS")
			} else {
				os.Setenv("SHM_COLLECT_SYSTEM_METRICS", tt.envValue)
			}
			defer os.Unsetenv("SHM_COLLECT_SYSTEM_METRICS")

			result := CollectSystemMetricsFromEnv()
			if result != tt.expected {
				t.Errorf("CollectSystemMetricsFromEnv() with env=%q = %v, want %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestDoNotTrack(t *testing.T) {
	tests := []struct {
		envValue string
		expected bool
	}{
		{"", false},         // absent = tracking allowed
		{"true", true},      // disabled
		{"TRUE", true},      // disabled
		{"1", true},         // disabled
		{"false", false},    // tracking allowed
		{"0", false},        // tracking allowed
		{"anything", false}, // unknown = tracking allowed
	}

	for _, tt := range tests {
		t.Run("DO_NOT_TRACK="+tt.envValue, func(t *testing.T) {
			if tt.envValue == "" {
				os.Unsetenv("DO_NOT_TRACK")
			} else {
				os.Setenv("DO_NOT_TRACK", tt.envValue)
			}
			defer os.Unsetenv("DO_NOT_TRACK")

			result := isDoNotTrack()
			if result != tt.expected {
				t.Errorf("isDoNotTrack() with env=%q = %v, want %v", tt.envValue, result, tt.expected)
			}
		})
	}
}

func TestClient_DoNotTrack_DisablesClient(t *testing.T) {
	tmpDir := t.TempDir()

	os.Setenv("DO_NOT_TRACK", "true")
	defer os.Unsetenv("DO_NOT_TRACK")

	cfg := Config{
		ServerURL:  "http://localhost:8080",
		AppName:    "test-app",
		AppVersion: "1.0.0",
		DataDir:    tmpDir,
		Enabled:    true, // explicitly enabled
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// DO_NOT_TRACK should override Enabled
	if client.config.Enabled != false {
		t.Errorf("config.Enabled = %v, want false when DO_NOT_TRACK=true", client.config.Enabled)
	}
}
