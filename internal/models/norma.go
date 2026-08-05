package models

import (
	"time"
)

// TipoNorma representa os tipos de normas do Banco Central
type TipoNorma string

const (
	TipoResolucao   TipoNorma = "Resolução"
	TipoCircular    TipoNorma = "Circular"
	TipoInstrucao   TipoNorma = "Instrução"
	TipoComunicado  TipoNorma = "Comunicado"
	TipoCartaCircular TipoNorma = "Carta-Circular"
	TipoOutros      TipoNorma = "Outros"
)

// Norma representa uma norma do Banco Central do Brasil
type Norma struct {
	ID            int64     `json:"id" db:"id"`
	Numero        string    `json:"numero" db:"numero"`
	Tipo          TipoNorma  `json:"tipo" db:"tipo"`
	Titulo        string    `json:"titulo" db:"titulo"`
	DataPublicacao string    `json:"data_publicacao" db:"data_publicacao"`
	DataVigencia  string    `json:"data_vigencia" db:"data_vigencia"`
	URL           string    `json:"url" db:"url"`
	Situacao      string    `json:"situacao" db:"situacao"`
	Assunto       string    `json:"assunto" db:"assunto"`
	Sumario       string    `json:"sumario" db:"sumario"`
	Texto         string    `json:"texto" db:"texto"`
	ArquivoPDF    string    `json:"arquivo_pdf" db:"arquivo_pdf"`
	CreatedAt     string    `json:"created_at" db:"created_at"`
	UpdatedAt     string    `json:"updated_at" db:"updated_at"`
}

// NormaSearch represent search criteria for normas
type NormaSearch struct {
	Tipo        *TipoNorma  `json:"tipo,omitempty"`
	Numero      *string    `json:"numero,omitempty"`
	Titulo      *string    `json:"titulo,omitempty"`
	DataDe      *time.Time `json:"data_de,omitempty"`
	DataAte     *time.Time `json:"data_ate,omitempty"`
	Situacao    *string    `json:"situacao,omitempty"`
	Assunto     *string    `json:"assunto,omitempty"`
	Page        int        `json:"page"`
	PageSize    int        `json:"page_size"`
}

// NormaResponse represents the API response for normas
type NormaResponse struct {
	Normas      []Norma `json:"normas"`
	Total       int    `json:"total"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
	TotalPages  int    `json:"total_pages"`
}

// Stats represents statistics about the database
type Stats struct {
	TotalNormas      int64 `json:"total_normas"`
	NormasVigentes   int64 `json:"normas_vigentes"`
	NormasRevogadas  int64 `json:"normas_revogadas"`
	Tipos            map[string]int64 `json:"tipos"`
	UltimaAtualizacao time.Time `json:"ultima_atualizacao"`
}
