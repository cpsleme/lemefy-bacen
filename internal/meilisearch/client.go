package meilisearch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/sirupsen/logrus"
)

const httpTimeout = 30 * time.Second

// defaultPrefix is the index prefix used when none is configured. Each collected
// tipo gets its own index named <prefix><slug> (e.g. "bcb_resolucao").
const defaultPrefix = "bcb_"

// allNormaTipos lists every tipo that gets a dedicated index.
func allNormaTipos() []models.TipoNorma {
	return []models.TipoNorma{
		models.TipoResolucao,
		models.TipoCircular,
		models.TipoInstrucao,
		models.TipoComunicado,
		models.TipoCartaCircular,
		models.TipoOutros,
	}
}

// slugTipo returns the lowercased, index-safe slug for a norma tipo.
func slugTipo(t models.TipoNorma) string {
	switch t {
	case models.TipoResolucao:
		return "resolucao"
	case models.TipoCircular:
		return "circular"
	case models.TipoInstrucao:
		return "instrucao"
	case models.TipoComunicado:
		return "comunicado"
	case models.TipoCartaCircular:
		return "carta_circular"
	default:
		return "outros"
	}
}

// Client is a minimal Meilisearch HTTP client used to mirror scraped normas
// into a local Meilisearch instance. Normas are routed to one index per tipo
// (Resolução, Circular, Instrução, Comunicado, Carta-Circular, Outros), each
// named <prefix><slug_tipo>. All write operations are best-effort and never
// fail the surrounding scrape when the server is unavailable.
type Client struct {
	cfg     *config.MeilisearchConfig
	logger  *logrus.Logger
	http    *http.Client
	baseURL string
	prefix  string
	// available tracks whether the remote server is reachable.
	available bool
}

// NewClient creates a new Meilisearch client and runs an initial health check.
func NewClient(cfg *config.MeilisearchConfig, logger *logrus.Logger) *Client {
	if logger == nil {
		logger = logrus.New()
	}
	if cfg == nil {
		cfg = &config.MeilisearchConfig{}
	}
	c := &Client{
		cfg:     cfg,
		logger:  logger,
		http:    &http.Client{Timeout: httpTimeout},
		baseURL: strings.TrimRight(cfg.Host, "/"),
		prefix:  cfg.IndexPrefix,
	}
	if strings.TrimSpace(c.prefix) == "" {
		c.prefix = defaultPrefix
	}
	if cfg.Enabled {
		c.available = c.healthCheck()
	}
	return c
}

// IndexFor returns the Meilisearch uid that a norma of the given tipo is
// written to, e.g. "bcb_resolucao" for Resolução.
func (c *Client) IndexFor(tipo models.TipoNorma) string {
	return c.prefix + slugTipo(tipo)
}

// IndexPrefix returns the configured index prefix.
func (c *Client) IndexPrefix() string {
	if c == nil {
		return ""
	}
	return c.prefix
}

// IndexUIDs returns the uids of every index this client manages (one per tipo).
func (c *Client) IndexUIDs() []string {
	if c == nil {
		return nil
	}
	uids := make([]string, 0, len(allNormaTipos()))
	for _, t := range allNormaTipos() {
		uids = append(uids, c.IndexFor(t))
	}
	return uids
}

// request builds and sends an HTTP request to the Meilisearch server.
func (c *Client) request(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	return c.http.Do(req)
}

// healthCheck probes the server and returns true when it is reachable.
func (c *Client) healthCheck() bool {
	resp, err := c.request(http.MethodGet, "/health", nil)
	if err != nil {
		c.logger.WithError(err).Warn("Meilisearch unavailable; indexing disabled")
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true
	}
	c.logger.Warnf("Meilisearch health check returned status %d", resp.StatusCode)
	return false
}

// createIndex creates a single index (idempotent against index_already_exists).
// Creation is asynchronous in Meilisearch, so the returned task is awaited to
// guarantee the index exists before callers try to write documents to it.
func (c *Client) createIndex(uid string) error {
	body, _ := json.Marshal(map[string]string{
		"uid":        uid,
		"primaryKey": "id",
	})
	resp, err := c.request(http.MethodPost, "/indexes", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create index %s: %w", uid, err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read create-index response for %s: %w", uid, err)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict {
		var errBody struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(respBytes, &errBody) == nil && errBody.Code == "index_already_exists" {
			return nil
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var task struct {
			TaskUID string `json:"taskUid"`
		}
		_ = json.Unmarshal(respBytes, &task)
		if task.TaskUID != "" {
			if err := c.awaitTask(task.TaskUID); err != nil {
				return fmt.Errorf("index %s creation: %w", uid, err)
			}
		}
		return nil
	}
	return fmt.Errorf("meilisearch index creation failed for %s with status %d: %s", uid, resp.StatusCode, string(respBytes))
}

// awaitTask polls a Meilisearch asynchronous task until it finishes, returning
// nil on success or an error if the task fails or does not complete in time.
func (c *Client) awaitTask(taskUID string) error {
	if taskUID == "" {
		return nil
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := c.request(http.MethodGet, "/tasks/"+url.PathEscape(taskUID), nil)
		if err != nil {
			return err
		}
		var t struct {
			Status string         `json:"status"`
			Error  map[string]any `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode task %s status: %w", taskUID, err)
		}
		resp.Body.Close()
		switch t.Status {
		case "succeeded":
			return nil
		case "failed":
			if t.Error != nil {
				return fmt.Errorf("meilisearch task %s failed: %v", taskUID, t.Error["message"])
			}
			return fmt.Errorf("meilisearch task %s failed", taskUID)
		default:
			time.Sleep(300 * time.Millisecond)
		}
	}
	return fmt.Errorf("meilisearch task %s did not complete within 60s", taskUID)
}

// EnsureIndex creates one index per collected tipo. Creating an index whose uid
// already exists is a no-op (Meilisearch returns index_already_exists, which is
// handled), so this is safe to call on every startup. A failure on one index is
// logged and skipped rather than aborting the whole set, so a single flaky index
// never blocks the others from being created or bulk-loaded.
func (c *Client) EnsureIndex() error {
	if !c.available {
		return nil
	}
	var errs []error
	for _, t := range allNormaTipos() {
		uid := c.IndexFor(t)
		if err := c.createIndex(uid); err != nil {
			c.logger.WithError(err).Warnf("Failed to ensure index %s", uid)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// IndexNorma upserts a single norma document into its target index (best effort).
// The document is keyed by its integer SQLite id, so updates replace the
// previous version. The request is enqueued asynchronously by Meilisearch.
func (c *Client) IndexNorma(norma *models.Norma) {
	if c == nil || !c.available || norma == nil {
		return
	}
	doc, err := json.Marshal(norma)
	if err != nil {
		c.logger.WithError(err).Warn("Failed to marshal norma for Meilisearch")
		return
	}
	uid := c.IndexFor(norma.Tipo)
	resp, err := c.request(http.MethodPost,
		"/indexes/"+uid+"/documents",
		bytes.NewReader(doc))
	if err != nil {
		c.logger.WithError(err).Debug("Failed to index norma into Meilisearch")
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		c.logger.Debugf("Meilisearch document upsert failed for id %d (status %d)", norma.ID, resp.StatusCode)
	}
}

// IndexNormas upserts a batch of normas, grouping documents by their target
// index so each index receives a single batched request.
func (c *Client) IndexNormas(normas []models.Norma) error {
	if c == nil || !c.available || len(normas) == 0 {
		return nil
	}
	groups := make(map[string][]models.Norma, len(allNormaTipos()))
	for _, n := range normas {
		uid := c.IndexFor(n.Tipo)
		groups[uid] = append(groups[uid], n)
	}
	for uid, bucket := range groups {
		body, err := json.Marshal(bucket)
		if err != nil {
			return fmt.Errorf("failed to marshal normas for index %s: %w", uid, err)
		}
		resp, err := c.request(http.MethodPost, "/indexes/"+uid+"/documents", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to batch index normas into %s: %w", uid, err)
		}
		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to read batch index response for %s: %w", uid, err)
		}
		if resp.StatusCode >= 300 {
			return fmt.Errorf("meilisearch batch index failed for %s (status %d): %s", uid, resp.StatusCode, string(respBytes))
		}
		var task struct {
			TaskUID string `json:"taskUid"`
		}
		if json.Unmarshal(respBytes, &task) == nil && task.TaskUID != "" {
			if err := c.awaitTask(task.TaskUID); err != nil {
				return fmt.Errorf("meilisearch batch index %s: %w", uid, err)
			}
		}
	}
	return nil
}

// NumberOfDocuments returns the document count for a given index uid.
func (c *Client) NumberOfDocuments(uid string) (int64, error) {
	if !c.available {
		return 0, nil
	}
	resp, err := c.request(http.MethodGet, "/indexes/"+uid+"/stats", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var stats struct {
		NumberOfDocuments int64 `json:"numberOfDocuments"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return 0, err
	}
	return stats.NumberOfDocuments, nil
}

// NeedsLoad reports whether any target index still needs its initial bulk load.
func (c *Client) NeedsLoad() bool {
	if !c.available {
		return false
	}
	for _, t := range allNormaTipos() {
		uid := c.IndexFor(t)
		if d, err := c.NumberOfDocuments(uid); err == nil && d == 0 {
			return true
		}
	}
	return false
}

// SyncAll bulk-loads every norma from storage, bucketing by tipo into the
// matching per-tipo index. An index is only loaded when it is empty, making the
// operation safe to run on every startup (idempotent and cheap when indices are
// already full).
func (c *Client) SyncAll(normas []models.Norma) (int, error) {
	if c == nil || !c.available || len(normas) == 0 {
		return 0, nil
	}

	buckets := make(map[string][]models.Norma, len(allNormaTipos()))
	for _, n := range normas {
		uid := c.IndexFor(n.Tipo)
		buckets[uid] = append(buckets[uid], n)
	}

	total := 0
	for _, t := range allNormaTipos() {
		uid := c.IndexFor(t)
		bucket := buckets[uid]
		if len(bucket) == 0 {
			continue
		}
		if d, _ := c.NumberOfDocuments(uid); d == 0 {
			if err := c.IndexNormas(bucket); err != nil {
				return total, fmt.Errorf("failed to bulk-load index %s: %w", uid, err)
			}
			total += len(bucket)
		}
	}
	return total, nil
}

// DeleteNormaByNorma removes a document from the index that corresponds to the
// given norma's type.
func (c *Client) DeleteNormaByNorma(norma *models.Norma) {
	if c == nil || !c.available || norma == nil || norma.ID == 0 {
		return
	}
	uid := c.IndexFor(norma.Tipo)
	resp, err := c.request(http.MethodDelete,
		"/indexes/"+uid+"/documents/"+url.PathEscape(fmt.Sprint(norma.ID)), nil)
	if err != nil {
		c.logger.WithError(err).Debug("Failed to delete norma from Meilisearch")
		return
	}
	resp.Body.Close()
}

// Available reports whether the Meilisearch server is reachable.
func (c *Client) Available() bool {
	if c == nil {
		return false
	}
	return c.available
}

// Enabled reports whether syncing to Meilisearch is enabled in configuration.
func (c *Client) Enabled() bool {
	if c == nil {
		return false
	}
	return c.cfg.Enabled
}

// Close releases underlying HTTP resources.
func (c *Client) Close() {
	if c == nil {
		return
	}
	c.http.CloseIdleConnections()
}
