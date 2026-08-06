package storage

import (
	"path/filepath"
	"testing"

	"github.com/lemefy/lemefy-bacen/internal/models"
)

func TestNewDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	if db == nil {
		t.Fatal("NewDatabase returned nil")
	}
}

func TestSaveAndGetNorma(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	norma := &models.Norma{
		Numero:        "123",
		Tipo:          models.TipoResolucao,
		Titulo:        "Test Norm",
		DataPublicacao: "2024-01-15T00:00:00Z",
		DataVigencia:  "2024-01-15T00:00:00Z",
		URL:           "https://example.com/norma/123",
		Situacao:      "Vigente",
		Assunto:       "Test subject",
	}

	err = db.SaveNorma(norma)
	if err != nil {
		t.Fatalf("SaveNorma failed: %v", err)
	}

	if norma.ID == 0 {
		t.Error("expected norma to have an ID after save")
	}

	retrieved, err := db.GetNormaByID(norma.ID)
	if err != nil {
		t.Fatalf("GetNormaByID failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected norma to be found")
	}
	if retrieved.Numero != norma.Numero {
		t.Errorf("expected Numero %s, got %s", norma.Numero, retrieved.Numero)
	}
	if retrieved.Titulo != norma.Titulo {
		t.Errorf("expected Titulo %s, got %s", norma.Titulo, retrieved.Titulo)
	}
}

func TestListNormas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	for i := 1; i <= 5; i++ {
		norma := &models.Norma{
			Numero:        string(rune('0' + i)),
			Tipo:          models.TipoResolucao,
			Titulo:        "Test Norm",
			DataPublicacao: "2024-01-15T00:00:00Z",
			DataVigencia:  "2024-01-15T00:00:00Z",
			URL:           "https://example.com/norma/" + string(rune('0'+i)),
			Situacao:      "Vigente",
		}
		if err := db.SaveNorma(norma); err != nil {
			t.Fatalf("SaveNorma failed: %v", err)
		}
	}

	search := &models.NormaSearch{
		Page:     1,
		PageSize: 10,
	}

	normas, total, err := db.ListNormas(search)
	if err != nil {
		t.Fatalf("ListNormas failed: %v", err)
	}

	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(normas) != 5 {
		t.Errorf("expected 5 normas, got %d", len(normas))
	}
}

func TestListNormasWithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	normas := []*models.Norma{
		{Numero: "1", Tipo: models.TipoResolucao, Titulo: "Resolution 1", DataPublicacao: "2024-01-15T00:00:00Z", DataVigencia: "2024-01-15T00:00:00Z", URL: "https://example.com/1", Situacao: "Vigente"},
		{Numero: "2", Tipo: models.TipoCircular, Titulo: "Circular 2", DataPublicacao: "2024-02-15T00:00:00Z", DataVigencia: "2024-02-15T00:00:00Z", URL: "https://example.com/2", Situacao: "Vigente"},
		{Numero: "3", Tipo: models.TipoResolucao, Titulo: "Resolution 3", DataPublicacao: "2024-03-15T00:00:00Z", DataVigencia: "2024-03-15T00:00:00Z", URL: "https://example.com/3", Situacao: "Revogada"},
	}

	for _, n := range normas {
		if err := db.SaveNorma(n); err != nil {
			t.Fatalf("SaveNorma failed: %v", err)
		}
	}

	tipo := models.TipoResolucao
	search := &models.NormaSearch{
		Page:    1,
		PageSize: 10,
		Tipo:    &tipo,
	}

	results, total, err := db.ListNormas(search)
	if err != nil {
		t.Fatalf("ListNormas failed: %v", err)
	}

	if total != 2 {
		t.Errorf("expected total 2 for Resolucao, got %d", total)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestGetStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalNormas != 0 {
		t.Errorf("expected TotalNormas 0, got %d", stats.TotalNormas)
	}
}

func TestSaveScrapeHistory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	err = db.SaveScrapeHistory(10, 5, 3, 1000, "completed", "")
	if err != nil {
		t.Fatalf("SaveScrapeHistory failed: %v", err)
	}

	history, err := db.GetLatestScrapeHistory()
	if err != nil {
		t.Fatalf("GetLatestScrapeHistory failed: %v", err)
	}

	if history == nil {
		t.Fatal("expected history to be non-nil")
	}
}

func TestDeleteOldNormas(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	oldNorma := &models.Norma{
		Numero:        "old",
		Tipo:          models.TipoResolucao,
		Titulo:        "Old Norm",
		DataPublicacao: "2020-01-01T00:00:00Z",
		DataVigencia:  "2020-01-01T00:00:00Z",
		URL:           "https://example.com/old",
		Situacao:      "Revogada",
	}

	err = db.SaveNorma(oldNorma)
	if err != nil {
		t.Fatalf("SaveNorma failed: %v", err)
	}

	deleted, err := db.DeleteOldNormas(365)
	if err != nil {
		t.Fatalf("DeleteOldNormas failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}
}

func TestCheckNormaExists(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	norma := &models.Norma{
		Numero: "1",
		Tipo:   models.TipoResolucao,
		Titulo: "Test",
		DataPublicacao: "2024-01-15T00:00:00Z",
		DataVigencia:  "2024-01-15T00:00:00Z",
		URL:           "https://example.com/test",
		Situacao:      "Vigente",
	}

	err = db.SaveNorma(norma)
	if err != nil {
		t.Fatalf("SaveNorma failed: %v", err)
	}

	exists, err := db.CheckNormaExists("https://example.com/test")
	if err != nil {
		t.Fatalf("CheckNormaExists failed: %v", err)
	}
	if !exists {
		t.Error("expected norma to exist")
	}

	exists, err = db.CheckNormaExists("https://example.com/nonexistent")
	if err != nil {
		t.Fatalf("CheckNormaExists failed: %v", err)
	}
	if exists {
		t.Error("expected norma to not exist")
	}
}

func TestGetNormaByURL(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	norma := &models.Norma{
		Numero: "1",
		Tipo:   models.TipoResolucao,
		Titulo: "Test",
		DataPublicacao: "2024-01-15T00:00:00Z",
		DataVigencia:  "2024-01-15T00:00:00Z",
		URL:           "https://example.com/test",
		Situacao:      "Vigente",
	}

	err = db.SaveNorma(norma)
	if err != nil {
		t.Fatalf("SaveNorma failed: %v", err)
	}

	retrieved, err := db.GetNormaByURL("https://example.com/test")
	if err != nil {
		t.Fatalf("GetNormaByURL failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected norma to be found")
	}
	if retrieved.Numero != "1" {
		t.Errorf("expected Numero '1', got %s", retrieved.Numero)
	}
}

func TestGetNormaByIDNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	retrieved, err := db.GetNormaByID(9999)
	if err != nil {
		t.Fatalf("GetNormaByID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil for non-existent norma")
	}
}

func TestGetNormaCountByDate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	norma := &models.Norma{
		Numero: "1",
		Tipo:   models.TipoResolucao,
		Titulo: "Test",
		DataPublicacao: "2024-01-15T00:00:00Z",
		DataVigencia:  "2024-01-15T00:00:00Z",
		URL:           "https://example.com/1",
		Situacao:      "Vigente",
	}

	err = db.SaveNorma(norma)
	if err != nil {
		t.Fatalf("SaveNorma failed: %v", err)
	}

	counts, err := db.GetNormaCountByDate()
	if err != nil {
		t.Fatalf("GetNormaCountByDate failed: %v", err)
	}

	if len(counts) == 0 {
		t.Error("expected at least one date count")
	}
}