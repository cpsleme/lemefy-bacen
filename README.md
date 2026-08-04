# lemfey-bacen

A CLI tool and web scraper for fetching and managing Banco Central do Brasil (BCB) normative documents.

## Overview

lemfey-bacen scrapes normative documents (Resoluções, Circulares, Instruções, Comunicados, Cartas-Circulares, and others) from the Banco Central do Brasil website and stores them in a SQLite database. It provides both a CLI interface and an HTTP API for querying the stored norms.

## Features

- **Web Scraping**: Automatically fetches norms from the BCB website
- **SQLite Storage**: Persistent storage of all scraped norms
- **HTTP API**: RESTful API for querying norms and statistics
- **Scheduled Updates**: Automatic periodic scraping and cleanup
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
| `LEMEFY_SCRAPER_BASE_URL` | https://www.bcb.gov.br/normativos | BCB base URL |
| `LEMEFY_SCRAPER_TIMEOUT` | 30 | Request timeout in seconds |
| `LEMEFY_SCHEDULER_ENABLED` | true | Enable automatic scheduling |
| `LEMEFY_LOGGING_LEVEL` | info | Log level |

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