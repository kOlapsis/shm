// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/kolapsis/shm/pkg/crypto"
)

type Config struct {
	ServerURL            string
	AppName              string
	AppVersion           string
	DataDir              string // where is store shm_identity.json
	Environment          string // prod, staging, ...
	Enabled              bool
	ReportInterval       time.Duration // snapshots interval (default: 1h)
	CollectSystemMetrics bool          // collect OS/runtime metrics (env: SHM_COLLECT_SYSTEM_METRICS)
}

type MetricsProvider func() map[string]interface{}

type Client struct {
	config    Config
	identity  *Identity
	provider  MetricsProvider
	client    *http.Client
	startTime time.Time
}

func New(cfg Config) (*Client, error) {
	if cfg.DataDir == "" {
		cfg.DataDir = "."
	}

	if cfg.ReportInterval == 0 {
		cfg.ReportInterval = 1 * time.Hour
	} else if cfg.ReportInterval < time.Minute {
		cfg.ReportInterval = time.Minute
	}

	if isDoNotTrack() {
		cfg.Enabled = false
	}

	if !cfg.Enabled {
		return &Client{config: cfg}, nil
	}

	ensureDataDir(cfg.DataDir)
	idPath := cfg.DataDir + "/shm_identity.json"
	id, err := loadOrGenerateIdentity(idPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init identity: %w", err)
	}

	return &Client{
		config:   cfg,
		identity: id,
		client:   &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *Client) SetProvider(p MetricsProvider) {
	c.provider = p
}

func (c *Client) Start(ctx context.Context) {
	if !c.config.Enabled {
		log.Println("[SHM] Telemetry disabled")
		return
	}

	if err := c.register(); err != nil {
		log.Printf("[SHM] Register warning: %v", err)
	}

	if err := c.activate(); err != nil {
		log.Printf("[SHM] Activation failed: %v", err)
	}

	ticker := time.NewTicker(c.config.ReportInterval)
	defer ticker.Stop()

	c.sendSnapshot()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sendSnapshot()
		}
	}
}

func (c *Client) register() error {
	req := RegisterRequest{
		InstanceID:  c.identity.InstanceID,
		PublicKey:   c.identity.PublicKey,
		AppName:     c.config.AppName,
		AppVersion:  c.config.AppVersion,
		Environment: c.config.Environment,
		OSArch:      runtime.GOOS + "/" + runtime.GOARCH,
	}

	body, _ := json.Marshal(req)
	resp, err := c.client.Post(c.config.ServerURL+"/v1/register", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	log.Printf("[SHM] Instance registered: %s", c.identity.InstanceID)
	return nil
}

func (c *Client) activate() error {
	payload := map[string]string{"action": "activate"}
	body, _ := json.Marshal(payload)

	privBytes, _ := hex.DecodeString(c.identity.PrivateKey)
	signature := crypto.Sign(privBytes, body)

	req, _ := http.NewRequest("POST", c.config.ServerURL+"/v1/activate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Instance-ID", c.identity.InstanceID)
	req.Header.Set("X-Signature", signature)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("activation failed: code %d", resp.StatusCode)
	}

	log.Printf("[SHM] Instance ACTIVATED successfully")
	return nil
}

func (c *Client) sendSnapshot() {
	data := make(map[string]interface{})
	if c.provider != nil {
		data = c.provider()
	}
	if c.config.CollectSystemMetrics {
		sysData := c.getSystemMetrics()
		for k, v := range sysData {
			data[k] = v
		}
	}
	metricsJSON, _ := json.Marshal(data)

	payload := SnapshotRequest{
		InstanceID: c.identity.InstanceID,
		Timestamp:  time.Now().UTC(),
		Metrics:    metricsJSON,
	}
	payloadBytes, _ := json.Marshal(payload)

	privBytes, _ := hex.DecodeString(c.identity.PrivateKey)
	signature := crypto.Sign(privBytes, payloadBytes)

	req, _ := http.NewRequest("POST", c.config.ServerURL+"/v1/snapshot", bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Instance-ID", c.identity.InstanceID)
	req.Header.Set("X-Signature", signature)

	resp, err := c.client.Do(req)
	if err != nil {
		log.Printf("[SHM] Failed to send snapshot: %v", err)
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		log.Printf("[SHM] Snapshot rejected: %d", resp.StatusCode)
	} else {
		log.Printf("[SHM] Snapshot sent successfully")
	}
}

func (c *Client) getSystemMetrics() map[string]interface{} {
	m := make(map[string]interface{})

	m["sys_os"] = runtime.GOOS
	m["sys_arch"] = runtime.GOARCH
	m["sys_cpu_cores"] = runtime.NumCPU()
	m["sys_go_version"] = runtime.Version()
	m["sys_mode"] = detectDeploymentMode()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	m["app_mem_alloc_mb"] = bytesToMB(mem.Alloc)
	m["app_goroutines"] = runtime.NumGoroutine()

	if !c.startTime.IsZero() {
		m["app_uptime_h"] = int(time.Since(c.startTime).Hours())
	}

	return m
}

func bytesToMB(b uint64) uint64 {
	return b / 1024 / 1024
}

func detectDeploymentMode() string {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_PORT") != "" {
		return "kubernetes"
	}
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount"); err == nil {
		return "kubernetes"
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	if isContainerCgroup() {
		return "docker"
	}
	return "standalone"
}

func isContainerCgroup() bool {
	data, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "docker") ||
		strings.Contains(content, "lxc") ||
		strings.Contains(content, "containerd")
}

func ensureDataDir(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}
}

func CollectSystemMetricsFromEnv() bool {
	val := strings.ToLower(os.Getenv("SHM_COLLECT_SYSTEM_METRICS"))
	return val != "false" && val != "0"
}

func isDoNotTrack() bool {
	val := strings.ToLower(os.Getenv("DO_NOT_TRACK"))
	return val == "true" || val == "1"
}

func slug(s string) string {
	s = strings.ToLower(s)

	replacements := map[rune]string{
		'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a",
		'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
		'ì': "i", 'í': "i", 'î': "i", 'ï': "i",
		'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o",
		'ù': "u", 'ú': "u", 'û': "u", 'ü': "u",
		'ý': "y", 'ÿ': "y",
		'ñ': "n", 'ç': "c",
	}

	var result strings.Builder
	for _, r := range s {
		if replacement, ok := replacements[r]; ok {
			result.WriteString(replacement)
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			result.WriteRune('-')
		}
	}

	cleaned := strings.Trim(result.String(), "-")
	for strings.Contains(cleaned, "--") {
		cleaned = strings.ReplaceAll(cleaned, "--", "-")
	}

	if cleaned == "" {
		return "app"
	}

	return cleaned
}
