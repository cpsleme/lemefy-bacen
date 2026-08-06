package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/models"
	"github.com/lemefy/lemefy-bacen/internal/parsers"
	_ "github.com/mattn/go-sqlite3"
)

// Database represents the SQLite database connection
type Database struct {
	conn *sql.DB
	path string
}

func unmarshalJSONString(raw string, v interface{}) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), v); err == nil {
		return nil
	}
	switch target := v.(type) {
	case *[]models.DocumentoPDF:
		*target = parsers.ParseDocumentosPDF(raw)
	case *[]models.NormaVinculada:
		*target = parsers.ParseNormasVinculadas(raw)
	case *[]models.Referencia:
		*target = parsers.ParseReferencias(raw)
	default:
		return fmt.Errorf("unsupported target type for legacy parse: %T", v)
	}
	return nil
}

// NewDatabase creates a new database instance
func NewDatabase(dbPath string) (*Database, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Set busy timeout first
	if _, err := conn.Exec("PRAGMA busy_timeout = 30000"); err != nil {
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Enable WAL mode for better performance
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		// If WAL fails, try DELETE mode
		if _, err2 := conn.Exec("PRAGMA journal_mode = DELETE"); err2 != nil {
			return nil, fmt.Errorf("failed to set journal mode: %w (WAL failed: %v)", err2, err)
		}
	}

	db := &Database{
		conn: conn,
		path: dbPath,
	}

	// Initialize database schema
	if err := db.InitSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	if db.conn != nil {
		return db.conn.Close()
	}
	return nil
}

// InitSchema initializes the database schema
func (db *Database) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS normas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		numero TEXT NOT NULL,
		tipo TEXT NOT NULL,
		titulo TEXT NOT NULL,
		data_publicacao TEXT NOT NULL,
		data_vigencia TEXT NOT NULL,
		url TEXT UNIQUE NOT NULL,
		situacao TEXT NOT NULL DEFAULT 'Vigente',
		assunto TEXT,
		texto TEXT,
		arquivo_pdf TEXT,
		documentos TEXT,
		dou TEXT,
		normas_vinculadas TEXT,
		referencias TEXT,
		atualizacoes TEXT,
		data_assinatura TEXT,
		voto TEXT,
		versao_normativo TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(numero, tipo, data_publicacao)
	);

	CREATE TABLE IF NOT EXISTS scrape_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TEXT NOT NULL DEFAULT (datetime('now')),
		normas_found INTEGER NOT NULL DEFAULT 0,
		normas_added INTEGER NOT NULL DEFAULT 0,
		normas_updated INTEGER NOT NULL DEFAULT 0,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'completed',
		error_message TEXT
	);

	CREATE TABLE IF NOT EXISTS stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		total_normas INTEGER NOT NULL DEFAULT 0,
		normas_vigentes INTEGER NOT NULL DEFAULT 0,
		normas_revogadas INTEGER NOT NULL DEFAULT 0,
		tipos JSON NOT NULL DEFAULT '{}',
		ultima_atualizacao TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_normas_url ON normas(url);
	CREATE INDEX IF NOT EXISTS idx_normas_tipo ON normas(tipo);
	CREATE INDEX IF NOT EXISTS idx_normas_data_publicacao ON normas(data_publicacao);
	CREATE INDEX IF NOT EXISTS idx_normas_data_vigencia ON normas(data_vigencia);
	CREATE INDEX IF NOT EXISTS idx_normas_situacao ON normas(situacao);
	CREATE INDEX IF NOT EXISTS idx_normas_numero ON normas(numero);

	CREATE TRIGGER IF NOT EXISTS update_normas_timestamp 
	AFTER UPDATE ON normas 
	FOR EACH ROW 
	BEGIN 
		UPDATE normas SET updated_at = datetime('now') WHERE id = OLD.id;
	END;
	`

	// Execute schema
	if _, err := db.conn.Exec(schema); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	// Migrate existing databases: drop columns removed from the model.
	columnsToDrop := []string{"sumario", "texto_url"}
	for _, col := range columnsToDrop {
		exists, err := db.columnExists("normas", col)
		if err != nil {
			return fmt.Errorf("failed to check column %s: %w", col, err)
		}
		if exists {
			if _, err := db.conn.Exec("ALTER TABLE normas DROP COLUMN " + col); err != nil {
				return fmt.Errorf("failed to drop column %s: %w", col, err)
			}
		}
	}

	return nil
}

// columnExists reports whether the given table has a column with the given name.
func (db *Database) columnExists(table, column string) (bool, error) {
	rows, err := db.conn.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// SaveNorma saves a norma to the database
func (db *Database) SaveNorma(norma *models.Norma) error {
	// Check if norma already exists by URL
	var existingID int64
	var existingNorma models.Norma
	var documentosJSON, normasVinculadasJSON, referenciasJSON string
	err := db.conn.QueryRow(
		"SELECT id, numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, COALESCE(texto, ''), arquivo_pdf, COALESCE(documentos, ''), COALESCE(dou, ''), COALESCE(normas_vinculadas, ''), COALESCE(referencias, ''), COALESCE(atualizacoes, ''), COALESCE(data_assinatura, ''), COALESCE(voto, ''), COALESCE(versao_normativo, ''), created_at, updated_at FROM normas WHERE url = ?",
		norma.URL,
	).Scan(
		&existingID, &existingNorma.Numero, &existingNorma.Tipo, &existingNorma.Titulo,
		&existingNorma.DataPublicacao, &existingNorma.DataVigencia, &existingNorma.URL,
		&existingNorma.Situacao, &existingNorma.Assunto, &existingNorma.Texto,
		&existingNorma.ArquivoPDF, &documentosJSON, &existingNorma.DOU, &normasVinculadasJSON, &referenciasJSON,
		&existingNorma.Atualizacoes, &existingNorma.DataAssinatura, &existingNorma.Voto, &existingNorma.VersaoNormativo,
		&existingNorma.CreatedAt, &existingNorma.UpdatedAt,
	)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing norma: %w", err)
	}

	if err := unmarshalJSONString(documentosJSON, &existingNorma.Documentos); err != nil {
		return fmt.Errorf("failed to unmarshal documentos: %w", err)
	}
	if err := unmarshalJSONString(normasVinculadasJSON, &existingNorma.NormasVinculadas); err != nil {
		return fmt.Errorf("failed to unmarshal normas_vinculadas: %w", err)
	}
	if err := unmarshalJSONString(referenciasJSON, &existingNorma.Referencias); err != nil {
		return fmt.Errorf("failed to unmarshal referencias: %w", err)
	}

	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	documentosJSONBytes, _ := json.Marshal(norma.Documentos)
	documentosJSON = string(documentosJSONBytes)
	normasVinculadasJSONBytes, _ := json.Marshal(norma.NormasVinculadas)
	normasVinculadasJSON = string(normasVinculadasJSONBytes)
	referenciasJSONBytes, _ := json.Marshal(norma.Referencias)
	referenciasJSON = string(referenciasJSONBytes)

	if existingID > 0 {
		// Update existing norma
		_, err = db.conn.Exec(
			"UPDATE normas SET numero = ?, tipo = ?, titulo = ?, data_publicacao = ?, data_vigencia = ?, situacao = ?, assunto = ?, texto = ?, arquivo_pdf = ?, documentos = ?, dou = ?, normas_vinculadas = ?, referencias = ?, atualizacoes = ?, data_assinatura = ?, voto = ?, versao_normativo = ?, updated_at = ? WHERE id = ?",
			norma.Numero, string(norma.Tipo), norma.Titulo, norma.DataPublicacao, norma.DataVigencia,
			norma.Situacao, norma.Assunto, norma.Texto, norma.ArquivoPDF,
			string(documentosJSON), norma.DOU, string(normasVinculadasJSON), string(referenciasJSON), norma.Atualizacoes,
			norma.DataAssinatura, norma.Voto, norma.VersaoNormativo, now, existingID,
		)
		if err != nil {
			return fmt.Errorf("failed to update norma: %w", err)
		}
		norma.ID = existingID
		norma.CreatedAt = existingNorma.CreatedAt
	} else {
		// Insert new norma
		result, err := db.conn.Exec(
			"INSERT INTO normas (numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, texto, arquivo_pdf, documentos, dou, normas_vinculadas, referencias, atualizacoes, data_assinatura, voto, versao_normativo, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			norma.Numero, string(norma.Tipo), norma.Titulo, norma.DataPublicacao, norma.DataVigencia,
			norma.URL, norma.Situacao, norma.Assunto, norma.Texto, norma.ArquivoPDF,
			string(documentosJSON), norma.DOU, string(normasVinculadasJSON), string(referenciasJSON), norma.Atualizacoes,
			norma.DataAssinatura, norma.Voto, norma.VersaoNormativo,
			now, now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert norma: %w", err)
		}

		id, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert id: %w", err)
		}
		norma.ID = id
		norma.CreatedAt = now
	}

	norma.UpdatedAt = now
	return nil
}

// GetNormaByID retrieves a norma by ID
func (db *Database) GetNormaByID(id int64) (*models.Norma, error) {
	var norma models.Norma
	var documentosJSON, normasVinculadasJSON, referenciasJSON string
	err := db.conn.QueryRow(
		"SELECT id, numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, COALESCE(texto, ''), arquivo_pdf, COALESCE(documentos, ''), COALESCE(dou, ''), COALESCE(normas_vinculadas, ''), COALESCE(referencias, ''), COALESCE(atualizacoes, ''), COALESCE(data_assinatura, ''), COALESCE(voto, ''), COALESCE(versao_normativo, ''), created_at, updated_at FROM normas WHERE id = ?",
		id,
	).Scan(
		&norma.ID, &norma.Numero, &norma.Tipo, &norma.Titulo,
		&norma.DataPublicacao, &norma.DataVigencia, &norma.URL,
		&norma.Situacao, &norma.Assunto, &norma.Texto,
		&norma.ArquivoPDF, &documentosJSON, &norma.DOU, &normasVinculadasJSON, &referenciasJSON,
		&norma.Atualizacoes, &norma.DataAssinatura, &norma.Voto, &norma.VersaoNormativo,
		&norma.CreatedAt, &norma.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get norma by id: %w", err)
	}

	if err := unmarshalJSONString(documentosJSON, &norma.Documentos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal documentos: %w", err)
	}
	if err := unmarshalJSONString(normasVinculadasJSON, &norma.NormasVinculadas); err != nil {
		return nil, fmt.Errorf("failed to unmarshal normas_vinculadas: %w", err)
	}
	if err := unmarshalJSONString(referenciasJSON, &norma.Referencias); err != nil {
		return nil, fmt.Errorf("failed to unmarshal referencias: %w", err)
	}

	return &norma, nil
}

// GetNormaByURL retrieves a norma by URL
func (db *Database) GetNormaByURL(url string) (*models.Norma, error) {
	var norma models.Norma
	var documentosJSON, normasVinculadasJSON, referenciasJSON string
	err := db.conn.QueryRow(
		"SELECT id, numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, COALESCE(texto, ''), arquivo_pdf, COALESCE(documentos, ''), COALESCE(dou, ''), COALESCE(normas_vinculadas, ''), COALESCE(referencias, ''), COALESCE(atualizacoes, ''), COALESCE(data_assinatura, ''), COALESCE(voto, ''), COALESCE(versao_normativo, ''), created_at, updated_at FROM normas WHERE url = ?",
		url,
	).Scan(
		&norma.ID, &norma.Numero, &norma.Tipo, &norma.Titulo,
		&norma.DataPublicacao, &norma.DataVigencia, &norma.URL,
		&norma.Situacao, &norma.Assunto, &norma.Texto,
		&norma.ArquivoPDF, &documentosJSON, &norma.DOU, &normasVinculadasJSON, &referenciasJSON,
		&norma.Atualizacoes, &norma.DataAssinatura, &norma.Voto, &norma.VersaoNormativo,
		&norma.CreatedAt, &norma.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get norma by url: %w", err)
	}

	if err := unmarshalJSONString(documentosJSON, &norma.Documentos); err != nil {
		return nil, fmt.Errorf("failed to unmarshal documentos: %w", err)
	}
	if err := unmarshalJSONString(normasVinculadasJSON, &norma.NormasVinculadas); err != nil {
		return nil, fmt.Errorf("failed to unmarshal normas_vinculadas: %w", err)
	}
	if err := unmarshalJSONString(referenciasJSON, &norma.Referencias); err != nil {
		return nil, fmt.Errorf("failed to unmarshal referencias: %w", err)
	}

	return &norma, nil
}

// ListNormas retrieves normas with optional filters
func (db *Database) ListNormas(search *models.NormaSearch) ([]models.Norma, int, error) {
	query := "SELECT id, numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, COALESCE(texto, ''), arquivo_pdf, COALESCE(documentos, ''), COALESCE(dou, ''), COALESCE(normas_vinculadas, ''), COALESCE(referencias, ''), COALESCE(atualizacoes, ''), COALESCE(data_assinatura, ''), COALESCE(voto, ''), COALESCE(versao_normativo, ''), created_at, updated_at FROM normas"
	var conditions []string
	var args []interface{}

	if search.Tipo != nil {
		conditions = append(conditions, "tipo = ?")
		args = append(args, string(*search.Tipo))
	}

	if search.Numero != nil && *search.Numero != "" {
		conditions = append(conditions, "numero LIKE ?")
		args = append(args, "%"+*search.Numero+"%")
	}

	if search.Titulo != nil && *search.Titulo != "" {
		conditions = append(conditions, "titulo LIKE ?")
		args = append(args, "%"+*search.Titulo+"%")
	}

	if search.Assunto != nil && *search.Assunto != "" {
		conditions = append(conditions, "assunto LIKE ?")
		args = append(args, "%"+*search.Assunto+"%")
	}

	if search.Situacao != nil && *search.Situacao != "" {
		conditions = append(conditions, "situacao = ?")
		args = append(args, *search.Situacao)
	}

	if search.DataDe != nil {
		conditions = append(conditions, "data_publicacao >= ?")
		args = append(args, search.DataDe.UTC().Format(time.RFC3339))
	}

	if search.DataAte != nil {
		conditions = append(conditions, "data_publicacao <= ?")
		args = append(args, search.DataAte.UTC().Format(time.RFC3339))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := "SELECT COUNT(*) FROM normas"
	if len(conditions) > 0 {
		countQuery += " WHERE " + strings.Join(conditions, " AND ")
	}

	var total int
	err := db.conn.QueryRow(countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count normas: %w", err)
	}

	// Add pagination
	if search.PageSize > 0 {
		offset := (search.Page - 1) * search.PageSize
		query += fmt.Sprintf(" ORDER BY data_publicacao DESC, id DESC LIMIT %d OFFSET %d", search.PageSize, offset)
	} else {
		query += " ORDER BY data_publicacao DESC, id DESC"
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query normas: %w", err)
	}
	defer rows.Close()

	var normas []models.Norma
	for rows.Next() {
		var norma models.Norma
		var documentosJSON, normasVinculadasJSON, referenciasJSON string
		err := rows.Scan(
			&norma.ID, &norma.Numero, &norma.Tipo, &norma.Titulo,
			&norma.DataPublicacao, &norma.DataVigencia, &norma.URL,
			&norma.Situacao, &norma.Assunto, &norma.Texto,
			&norma.ArquivoPDF, &documentosJSON, &norma.DOU, &normasVinculadasJSON, &referenciasJSON,
			&norma.Atualizacoes, &norma.DataAssinatura, &norma.Voto, &norma.VersaoNormativo,
			&norma.CreatedAt, &norma.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan norma: %w", err)
		}

		if err := unmarshalJSONString(documentosJSON, &norma.Documentos); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal documentos: %w", err)
		}
		if err := unmarshalJSONString(normasVinculadasJSON, &norma.NormasVinculadas); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal normas_vinculadas: %w", err)
		}
		if err := unmarshalJSONString(referenciasJSON, &norma.Referencias); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal referencias: %w", err)
		}

		normas = append(normas, norma)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating normas: %w", err)
	}

	return normas, total, nil
}

// GetStats retrieves statistics about the database
func (db *Database) GetStats() (*models.Stats, error) {
	var stats models.Stats

	// Get total normas
	err := db.conn.QueryRow("SELECT COUNT(*) FROM normas").Scan(&stats.TotalNormas)
	if err != nil {
		return nil, fmt.Errorf("failed to get total normas: %w", err)
	}

	// Get vigentes
	var vigentes int64
	err = db.conn.QueryRow("SELECT COUNT(*) FROM normas WHERE situacao = 'Vigente'").Scan(&vigentes)
	if err != nil {
		return nil, fmt.Errorf("failed to get vigentes count: %w", err)
	}

	// Get revogadas
	var revogadas int64
	err = db.conn.QueryRow("SELECT COUNT(*) FROM normas WHERE situacao = 'Revogada'").Scan(&revogadas)
	if err != nil {
		return nil, fmt.Errorf("failed to get revogadas count: %w", err)
	}

	// Get tipos
	rows, err := db.conn.Query("SELECT tipo, COUNT(*) as count FROM normas GROUP BY tipo")
	if err != nil {
		return nil, fmt.Errorf("failed to get tipos: %w", err)
	}
	defer rows.Close()

	stats.Tipos = make(map[string]int64)
	for rows.Next() {
		var tipo string
		var count int64
		if err := rows.Scan(&tipo, &count); err != nil {
			return nil, fmt.Errorf("failed to scan tipo: %w", err)
		}
		stats.Tipos[tipo] = count
	}

	// Get last update
	var lastUpdate sql.NullString
	err = db.conn.QueryRow("SELECT MAX(updated_at) FROM normas").Scan(&lastUpdate)
	if err != nil {
		return nil, fmt.Errorf("failed to get last update: %w", err)
	}

	if lastUpdate.Valid && lastUpdate.String != "" {
		lastUpdateTime, err := time.Parse("2006-01-02 15:04:05", lastUpdate.String)
		if err != nil {
			stats.UltimaAtualizacao = time.Now().UTC()
		} else {
			stats.UltimaAtualizacao = lastUpdateTime.UTC()
		}
	} else {
		stats.UltimaAtualizacao = time.Now().UTC()
	}

	stats.NormasVigentes = vigentes
	stats.NormasRevogadas = revogadas

	return &stats, nil
}

// SaveScrapeHistory saves scrape history
func (db *Database) SaveScrapeHistory(found, added, updated, durationMS int, status, errorMsg string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := db.conn.Exec(
		"INSERT INTO scrape_history (timestamp, normas_found, normas_added, normas_updated, duration_ms, status, error_message) VALUES (?, ?, ?, ?, ?, ?, ?)",
		now, found, added, updated, durationMS, status, errorMsg,
	)
	return err
}

// ScrapeHistory represents a single scrape history record
type ScrapeHistory struct {
	Timestamp     string `json:"timestamp" db:"timestamp"`
	NormasFound   int    `json:"normas_found" db:"normas_found"`
	NormasAdded   int    `json:"normas_added" db:"normas_added"`
	NormasUpdated int    `json:"normas_updated" db:"normas_updated"`
	DurationMS    int    `json:"duration_ms" db:"duration_ms"`
	Status        string `json:"status" db:"status"`
	ErrorMessage  string `json:"error_message,omitempty" db:"error_message"`
}

// GetLatestScrapeHistory retrieves the latest scrape history
func (db *Database) GetLatestScrapeHistory() (map[string]interface{}, error) {
	var history ScrapeHistory
	err := db.conn.QueryRow(
		"SELECT timestamp, normas_found, normas_added, normas_updated, duration_ms, status, error_message FROM scrape_history ORDER BY timestamp DESC LIMIT 1",
	).Scan(
		&history.Timestamp, &history.NormasFound, &history.NormasAdded,
		&history.NormasUpdated, &history.DurationMS, &history.Status, &history.ErrorMessage,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest scrape history: %w", err)
	}

	// Convert to map for backward compatibility
	result := map[string]interface{}{
		"timestamp":      history.Timestamp,
		"normas_found":   history.NormasFound,
		"normas_added":   history.NormasAdded,
		"normas_updated": history.NormasUpdated,
		"duration_ms":    history.DurationMS,
		"status":         history.Status,
		"error_message":  history.ErrorMessage,
	}

	return result, nil
}

// DeleteOldNormas deletes normas older than the specified days
func (db *Database) DeleteOldNormas(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339)

	result, err := db.conn.Exec("DELETE FROM normas WHERE data_publicacao < ? AND situacao = 'Revogada'", cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old normas: %w", err)
	}

	return result.RowsAffected()
}

// CheckNormaExists checks if a norma exists by URL
func (db *Database) CheckNormaExists(url string) (bool, error) {
	var exists bool
	err := db.conn.QueryRow("SELECT EXISTS(SELECT 1 FROM normas WHERE url = ?)", url).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check norma exists: %w", err)
	}
	return exists, nil
}

// GetMaxDataPublicacao returns the most recent data_publicacao stored, as a
// date string in "2006-01-02" format, or "" when the database is empty.
func (db *Database) GetMaxDataPublicacao() (string, error) {
	var v sql.NullString
	err := db.conn.QueryRow("SELECT MAX(DATE(data_publicacao)) FROM normas").Scan(&v)
	if err != nil {
		return "", fmt.Errorf("failed to get max data_publicacao: %w", err)
	}
	if !v.Valid {
		return "", nil
	}
	return v.String, nil
}

// GetNormaCountByDate retrieves the count of normas by date
func (db *Database) GetNormaCountByDate() (map[string]int, error) {
	rows, err := db.conn.Query("SELECT DATE(data_publicacao) as date, COUNT(*) as count FROM normas GROUP BY DATE(data_publicacao) ORDER BY date DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to get norma count by date: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var date string
		var count int
		if err := rows.Scan(&date, &count); err != nil {
			return nil, fmt.Errorf("failed to scan date count: %w", err)
		}
		result[date] = count
	}

	return result, nil
}

// GetAllNormas retrieves every norma stored in the database. Used to perform
// an initial bulk load into the search index.
func (db *Database) GetAllNormas() ([]models.Norma, error) {
	rows, err := db.conn.Query(
		"SELECT id, numero, tipo, titulo, data_publicacao, data_vigencia, url, situacao, assunto, COALESCE(texto, ''), arquivo_pdf, COALESCE(documentos, ''), COALESCE(dou, ''), COALESCE(normas_vinculadas, ''), COALESCE(referencias, ''), COALESCE(atualizacoes, ''), COALESCE(data_assinatura, ''), COALESCE(voto, ''), COALESCE(versao_normativo, ''), created_at, updated_at FROM normas",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query all normas: %w", err)
	}
	defer rows.Close()

	var normas []models.Norma
	for rows.Next() {
		var norma models.Norma
		var documentosJSON, normasVinculadasJSON, referenciasJSON string
		if err := rows.Scan(
			&norma.ID, &norma.Numero, &norma.Tipo, &norma.Titulo,
			&norma.DataPublicacao, &norma.DataVigencia, &norma.URL,
			&norma.Situacao, &norma.Assunto, &norma.Texto,
			&norma.ArquivoPDF, &documentosJSON, &norma.DOU, &normasVinculadasJSON, &referenciasJSON,
			&norma.Atualizacoes, &norma.DataAssinatura, &norma.Voto, &norma.VersaoNormativo,
			&norma.CreatedAt, &norma.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan norma: %w", err)
		}

		if err := unmarshalJSONString(documentosJSON, &norma.Documentos); err != nil {
			return nil, fmt.Errorf("failed to unmarshal documentos: %w", err)
		}
		if err := unmarshalJSONString(normasVinculadasJSON, &norma.NormasVinculadas); err != nil {
			return nil, fmt.Errorf("failed to unmarshal normas_vinculadas: %w", err)
		}
		if err := unmarshalJSONString(referenciasJSON, &norma.Referencias); err != nil {
			return nil, fmt.Errorf("failed to unmarshal referencias: %w", err)
		}

		normas = append(normas, norma)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating normas: %w", err)
	}

	return normas, nil
}

// MigrateLegacyReferencias converts legacy pipe-separated referencias strings
// to JSON arrays. This ensures all referencias are stored as proper JSON arrays.
func (db *Database) MigrateLegacyReferencias() (int64, error) {
	rows, err := db.conn.Query(
		"SELECT id, referencias FROM normas WHERE referencias IS NOT NULL AND referencias != '' AND referencias != 'null' AND referencias NOT LIKE '[%'",
	)
	if err != nil {
		return 0, fmt.Errorf("failed to query legacy referencias: %w", err)
	}
	defer rows.Close()

	var count int64
	for rows.Next() {
		var id int64
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return count, fmt.Errorf("failed to scan legacy referencias: %w", err)
		}

		refs := parsers.ParseReferencias(raw)
		if len(refs) == 0 {
			continue
		}

		refsJSON, err := json.Marshal(refs)
		if err != nil {
			return count, fmt.Errorf("failed to marshal referencias for id %d: %w", id, err)
		}

		_, err = db.conn.Exec(
			"UPDATE normas SET referencias = ? WHERE id = ?",
			string(refsJSON), id,
		)
		if err != nil {
			return count, fmt.Errorf("failed to update referencias for id %d: %w", id, err)
		}
		count++
	}

	if err = rows.Err(); err != nil {
		return count, fmt.Errorf("error iterating legacy referencias: %w", err)
	}

	return count, nil
}
