package meilisearch

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/sirupsen/logrus"
)

const testDocID = "9999"

func meiliTestConfig(t *testing.T) *config.MeilisearchConfig {
	cfg := &config.MeilisearchConfig{
		Enabled:     true,
		Host:        "http://localhost:7700",
		IndexPrefix: "test_" + t.Name() + "_",
	}
	if v := os.Getenv("MEILISEARCH_HOST"); v != "" {
		cfg.Host = v
	}
	cfg.APIKey = os.Getenv("MEILISEARCH_API_KEY")
	return cfg
}

// newTestClient creates an isolated client backed by a per-tipo set of test
// indices. The indices are removed on cleanup so tests don't collide with
// production data in the real bcb_* indices.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	cfg := meiliTestConfig(t)
	if cfg.APIKey == "" {
		t.Skip("Meilisearch API key required for integration test (set MEILISEARCH_API_KEY)")
	}
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	c := NewClient(cfg, logger)
	if !c.Available() {
		t.Skip("Meilisearch not available on ", cfg.Host)
	}
	if err := c.EnsureIndex(); err != nil {
		t.Fatalf("EnsureIndex failed: %v", err)
	}
	t.Cleanup(func() {
		for _, uid := range c.IndexUIDs() {
			c.request(http.MethodDelete, "/indexes/"+uid, nil)
		}
		c.Close()
	})
	return c
}

// waitForDocument polls the document retrieval endpoint until the document
// reaches the expected found state or the deadline elapses.
func waitForDocument(t *testing.T, c *Client, uid, id string, wantFound bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := c.request(http.MethodGet, "/indexes/"+uid+"/documents/"+id, nil)
		if err == nil {
			found := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if found == wantFound {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("document %s in index %s was not in expected state (found=%v)", id, uid, wantFound)
}

func TestIndexAndRetrieveNorma(t *testing.T) {
	c := newTestClient(t)
	uid := c.IndexFor(models.TipoResolucao)

	norma := &models.Norma{
		ID:             9999,
		Numero:         "123-test",
		Tipo:           models.TipoResolucao,
		Titulo:         "Norma de teste automatizado",
		DataPublicacao: "2024-01-15T00:00:00Z",
		DataVigencia:   "2024-01-15T00:00:00Z",
		URL:            "https://example.com/test-norma-9999",
		Situacao:       "Vigente",
		Assunto:        "automated test",
		ArquivoPDF:     "https://example.com/test.pdf",
	}

	// Upsert (fire-and-forget; Meilisearch indexes asynchronously).
	c.IndexNorma(norma)
	waitForDocument(t, c, uid, testDocID, true)

	resp, err := c.request(http.MethodGet, "/indexes/"+uid+"/documents/"+testDocID, nil)
	if err != nil {
		t.Fatalf("failed to request document: %v", err)
	}
	var got models.Norma
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode document: %v", err)
	}
	resp.Body.Close()
	if got.Numero != norma.Numero || got.Titulo != norma.Titulo {
		t.Errorf("unexpected document content: got numero=%q titulo=%q, want numero=%q titulo=%q",
			got.Numero, got.Titulo, norma.Numero, norma.Titulo)
	}

	// Upsert an update (same id, new title) -> should replace, not duplicate.
	updated := *norma
	updated.Titulo = "Norma de teste atualizada"
	c.IndexNorma(&updated)

	deadline := time.Now().Add(10 * time.Second)
	updatedOK := false
	for time.Now().Before(deadline) {
		r, err := c.request(http.MethodGet, "/indexes/"+uid+"/documents/"+testDocID, nil)
		if err == nil && r.StatusCode == http.StatusOK {
			var got2 models.Norma
			if json.NewDecoder(r.Body).Decode(&got2) == nil && got2.Titulo == updated.Titulo {
				updatedOK = true
			}
			r.Body.Close()
			if updatedOK {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !updatedOK {
		t.Fatal("updated document was never indexed by Meilisearch")
	}

	// Delete and confirm it is gone.
	c.DeleteNormaByNorma(&updated)
	waitForDocument(t, c, uid, testDocID, false)
}

func TestSyncAllBulkLoad(t *testing.T) {
	c := newTestClient(t)

	normas := []models.Norma{
		{ID: 9997, Numero: "A1", Tipo: models.TipoCircular, Titulo: "Bulk alpha", URL: "https://example.com/bulk-a1", Situacao: "Vigente"},
		{ID: 9998, Numero: "B2", Tipo: models.TipoInstrucao, Titulo: "Bulk beta", URL: "https://example.com/bulk-b2", Situacao: "Vigente"},
	}

	n, err := c.SyncAll(normas)
	if err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 indexed, got %d", n)
	}
	waitForDocument(t, c, c.IndexFor(models.TipoCircular), "9997", true)
	waitForDocument(t, c, c.IndexFor(models.TipoInstrucao), "9998", true)
}

func TestIndexNormaRoutesEachTipoToOwnIndex(t *testing.T) {
	c := newTestClient(t)

	resolucao := &models.Norma{
		ID:     9995,
		Numero: "R1",
		Tipo:   models.TipoResolucao,
		Titulo: "Res test",
		URL:    "https://example.com/r1",
	}
	comunicado := &models.Norma{
		ID:     9996,
		Numero: "C1",
		Tipo:   models.TipoComunicado,
		Titulo: "Com test",
		URL:    "https://example.com/c1",
	}

	c.IndexNorma(resolucao)
	c.IndexNorma(comunicado)

	resUID := c.IndexFor(models.TipoResolucao)
	comUID := c.IndexFor(models.TipoComunicado)

	// Each tipo lands in its own dedicated index.
	waitForDocument(t, c, resUID, "9995", true)
	waitForDocument(t, c, comUID, "9996", true)

	// Routing is exclusive: a norma must never appear in another tipo's index.
	if resUID == comUID {
		t.Fatalf("expected distinct indices, got %q for both", resUID)
	}
	waitForDocument(t, c, resUID, "9996", false)
	waitForDocument(t, c, comUID, "9995", false)
}

func TestIndexUIDsHasOneIndexPerTipo(t *testing.T) {
	cfg := &config.MeilisearchConfig{
		Enabled:     false,
		IndexPrefix: "bcb_",
	}
	c := NewClient(cfg, logrus.New())
	uids := c.IndexUIDs()
	want := []string{
		"bcb_resolucao", "bcb_circular", "bcb_instrucao",
		"bcb_comunicado", "bcb_carta_circular", "bcb_outros",
	}
	if len(uids) != len(want) {
		t.Fatalf("expected %d indices, got %d (%v)", len(want), len(uids), uids)
	}
	got := map[string]bool{}
	for _, u := range uids {
		got[u] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing index %q in %v", w, uids)
		}
	}
}
