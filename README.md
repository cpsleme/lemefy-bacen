# lemfey-bacen

A CLI tool and web scraper for fetching and managing Banco Central do Brasil (BCB) normative documents.

## Overview

lemfey-bacen scrapes normative documents (Resoluções, Circulares, Instruções, Comunicados, Cartas-Circulares, and others) from the Banco Central do Brasil website and stores them in a SQLite database. It provides both a CLI interface and an HTTP API for querying the stored norms.

## Features

- **Web Scraping**: Automatically fetches norms from the BCB website
- **SQLite Storage**: Persistent storage of all scraped norms
- **HTTP API**: RESTful API for querying norms and statistics
- **Scheduled Updates**: Automatic periodic scraping and cleanup
- **Meilisearch Sync**: Optionally mirrors every scraped norm into a local Meilisearch index for full-text search
- **CLI Interface**: Command-line tool for all operations

## Installation

```bash
go build -o lemfey-bacen ./cmd/cli
```

## Usage

### CLI Commands

```bash
# Show help
lemfey-bacen --help

# Run the scraper
lemfey-bacen scrape
lemfey-bacen scrape --tipo Resolução
lemfey-bacen scrape --recent --days 30

# Start the HTTP server
lemfey-bacen serve --port 8080

# Query norms
lemfey-bacen normas
lemfey-bacen normas --tipo Circular --page 2
lemfey-bacen normas --output json

# Show statistics
lemfey-bacen stats

# Manage scheduler
lemfey-bacen scheduler status
lemfey-bacen scheduler run-now
lemfey-bacen scheduler config

# Show configuration
lemfey-bacen config

# Show version
lemfey-bacen version
```

### HTTP API

When running with `serve`, the following endpoints are available:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/normas` | List norms with filters |
| GET | `/api/normas/{id}` | Get a specific norm |
| GET | `/api/stats` | Get database statistics |
| POST | `/api/scrape` | Trigger a scrape |
| GET | `/api/schedule` | Get scheduler info |
| POST | `/api/schedule` | Run immediate update |
| GET | `/api/health` | Health check |

### Query Parameters for `/api/normas`

- `page` - Page number (default: 1)
- `page_size` - Items per page (default: 50, max: 1000)
- `tipo` - Filter by norm type
- `numero` - Filter by norm number
- `titulo` - Filter by title
- `assunto` - Filter by subject
- `situacao` - Filter by situation
- `data_de` - Filter by publication date from (YYYY-MM-DD)
- `data_ate` - Filter by publication date until (YYYY-MM-DD)

## Configuration

Configuration is loaded from `config.yaml` in the current directory or from environment variables with the `LEMEFY_` prefix.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LEMEFY_APP_PORT` | 8080 | HTTP server port |
| `LEMEFY_DATABASE_PATH` | data/normas.db | SQLite database path |
 | `LEMEFY_MEILISEARCH_ENABLED` | true | Enable syncing scraped norms to a local Meilisearch |
 | `LEMEFY_MEILISEARCH_HOST` | http://localhost:7700 | Meilisearch server URL |
 | `LEMEFY_MEILISEARCH_API_KEY` | | Meilisearch master key (required if the server has authentication enabled) |
 | `LEMEFY_MEILISEARCH_INDEX_PREFIX` | bcb_ | Prefix for Meilisearch index names; a dedicated index is created per norma type (`{prefix}{slug}`, e.g. `bcb_resolucao`, `bcb_comunicado`) |
 | `LEMEFY_SCRAPER_BASE_URL` | https://www.bcb.gov.br/normativos | BCB base URL |
| `LEMEFY_SCRAPER_TIMEOUT` | 30 | Request timeout in seconds |
| `LEMEFY_SCHEDULER_ENABLED` | true | Enable automatic scheduling |
| `LEMEFY_LOGGING_LEVEL` | info | Log level |

### Meilisearch Sync

When `LEMEFY_MEILISEARCH_ENABLED` is `true` (default), every norma scraped or updated is mirrored into a local Meilisearch instance. Normas are routed to **one index per collectible type** (`<prefix><slug>`), so each tipo is independently searchable:

| Tipo | Index |
|------|-------|
| Resolução | `bcb_resolucao` |
| Circular | `bcb_circular` |
| Instrução | `bcb_instrucao` |
| Comunicado | `bcb_comunicado` |
| Carta-Circular | `bcb_carta_circular` |
| Outros | `bbc_outros` |

The prefix is configurable via `LEMEFY_MEILISEARCH_INDEX_PREFIX` (default `bcb_`). On startup, any existing normas from SQLite are bulk-loaded into the corresponding per-tipo index (only when that index is empty). The scraper also upserts each new/updated norma into its tipo index in real time as it is scraped.

Syncing is best-effort: if Meilisearch is unreachable, scrapes still succeed and failures are logged. If your Meilisearch instance was started with a master key (e.g. `./meilisearch --master-key="..."`), set the key via `LEMEFY_MEILISEARCH_API_KEY`:

```bash
# Start Meilisearch (with auth)
./meilisearch --master-key="aSampleMasterKey"

# Run with the project so scraped data is indexed
LEMEFY_MEILISEARCH_API_KEY=aSampleMasterKey go run ./cmd/cli scrape

# Query by tipo index
curl -H "Authorization: Bearer aSampleMasterKey" \
  -X POST http://localhost:7700/indexes/bcb_resolucao/search \
  -H 'Content-Type: application/json' \
  -d '{"q":"Resolução"}'
curl -H "Authorization: Bearer aSampleMasterKey" \
  -X POST http://localhost:7700/indexes/bcb_comunicado/search \
  -H 'Content-Type: application/json' \
  -d '{"q":"Comunicado"}'
```

## Project Structure

```
lemfey-bacen/
├── cmd/
│   ├── cli/          # CLI entry point and commands
│   │   ├── main.go
│   │   └── cmds/
│   │       ├── app.go
│   │       ├── config.go
│   │       ├── normas.go
│   │       ├── output.go
│   │       ├── root.go
│   │       ├── scheduler.go
│   │       ├── scrape.go
│   │       ├── serve.go
│   │       ├── stats.go
│   │       └── version.go
│   └── scraper/      # Original server entry point
│       └── main.go
├── internal/
│   ├── config/       # Configuration management
│   ├── meilisearch/  # Local Meilisearch sync client
│   ├── models/       # Data models
│   ├── scheduler/    # Scheduled task management
│   ├── scraper/      # Web scraping logic
│   └── storage/      # SQLite database operations
├── pkg/
│   ├── api/          # API utilities
│   └── utils/        # Shared utilities
├── go.mod
├── go.sum
└── README.md
```

## Development

### Building

```bash
# Build CLI
go build -o lemfey-bacen ./cmd/cli

# Build scraper server
go build -o scraper ./cmd/scraper
```

### Running Tests

```bash
go test ./...
```

### Running the CLI

```bash
go run ./cmd/cli scrape
go run ./cmd/cli serve
```

## License

MIT