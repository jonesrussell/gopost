# GoPost Integration Service

A Go service that bridges Elasticsearch (where crawled articles are stored) and Drupal 11 (via JSON:API), specifically designed for filtering and posting crime-related news articles.

## Architecture

```
┌─────────────────┐
│  Go Crawler     │
│  (existing)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐      ┌──────────────────┐
│ Elasticsearch   │◄─────┤ Go Integration   │
│  Indexes        │      │   Service        │
└─────────────────┘      │  (this service)  │
                         │                  │
                         │ - Query ES       │
                         │ - Filter crime   │
                         │ - POST to Drupal │
                         └────────┬─────────┘
                                  │
                                  ▼
                         ┌─────────────────┐
                         │  Drupal 11      │
                         │  JSON:API       │
                         └─────────────────┘
```

## Features

- **Elasticsearch Integration**: Queries ES for new articles based on crime keywords
- **Crime Article Filtering**: Uses keyword matching to identify crime-related content
- **Drupal JSON:API**: Posts filtered articles to Drupal via JSON:API
- **Deduplication**: Uses Redis to track already-posted articles
- **Rate Limiting**: Prevents overwhelming Drupal with requests
- **Multi-City Support**: Configure multiple cities with their own ES indexes and Drupal groups
- **Graceful Shutdown**: Handles SIGTERM/SIGINT for clean shutdowns

## Prerequisites

- Go 1.25 or later
- Task (taskfile.dev) - for running build tasks
- Elasticsearch 8.x
- Redis 6.x or later
- Drupal 11 with JSON:API enabled
- Drupal OAuth2 token for API authentication

## Quick Start

### 1. Configuration

Copy the example config file:

```bash
cp config.yml.example config.yml
```

Edit `config.yml` with your settings:

```yaml
elasticsearch:
  url: "http://localhost:9200"

drupal:
  url: "https://your-drupal-site.com"
  token: "your-oauth-token"

redis:
  url: "localhost:6379"

cities:
  - name: "sudbury_com"
    index: "sudbury_com_articles"
    group_id: "uuid-of-sudbury-group"
```

### 2. Environment Variables (Optional)

You can override config values with environment variables:

- `ES_URL` - Elasticsearch URL
- `DRUPAL_URL` - Drupal site URL
- `DRUPAL_TOKEN` - Drupal OAuth token
- `REDIS_URL` - Redis connection string

### 3. Install Task (if not already installed)

```bash
# macOS
brew install go-task/tap/go-task

# Linux
sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin

# Or via Go
go install github.com/go-task/task/v3/cmd/task@latest
```

### 4. Run Locally

```bash
# Install dependencies
task deps

# Run the service
task run

# Or build and run manually
task build
./bin/integration -config config.yml
```

### 4. Run with Docker Compose

```bash
# Set environment variables
export DRUPAL_URL=https://your-drupal-site.com
export DRUPAL_TOKEN=your-token

# Start all services
docker-compose up -d

# View logs
docker-compose logs -f integration
```

## Drupal Setup

### 1. Enable JSON:API

```bash
drush en jsonapi -y
```

### 2. Create OAuth2 Token

1. Install the OAuth2 module: `drush en oauth2 -y`
2. Create a client in Drupal admin
3. Generate a token for API access
4. Use this token in the `config.yml` file

### 3. Content Type Structure

The service expects a Drupal content type with:
- `title` field
- `body` field (or similar)
- `field_url` field (URL field)
- `field_group` field (entity reference to group)

### 4. Group Configuration

Create groups in Drupal (e.g., "Sudbury, Ontario, Canada - Crime News") and note their UUIDs. Use these UUIDs in the `cities` configuration.

## Configuration Reference

### Service Settings

- `check_interval`: How often to check for new articles (e.g., "5m", "1h")
- `rate_limit_rps`: Maximum requests per second to Drupal
- `lookback_hours`: How many hours back to search in Elasticsearch
- `crime_keywords`: List of keywords to identify crime articles
- `content_type`: Drupal content type (default: "node--article")
- `group_type`: Drupal group type (default: "group--crime_news")

### City Configuration

Each city requires:
- `name`: City identifier (used for logging)
- `index`: Elasticsearch index name (optional, defaults to `{name}_articles`)
- `group_id`: Drupal group UUID where articles should be posted

## Elasticsearch Article Schema

The service expects articles in Elasticsearch with the following structure:

```json
{
  "id": "article-123",
  "title": "Police arrest suspect in downtown area",
  "content": "Full article content...",
  "url": "https://example.com/article",
  "published_at": "2024-01-15T10:30:00Z",
  "source": "example.com"
}
```

## Development

### Available Tasks

```bash
# Show all available tasks
task

# Build the service
task build

# Run the service
task run

# Run tests
task test

# Run tests with coverage
task test:coverage

# Run tests with race detector
task test:race

# Format code
task fmt

# Run code quality checks (fmt, vet, lint)
task check

# Clean build artifacts
task clean

# Download and tidy dependencies
task deps

# Docker commands
task docker:build
task docker:up
task docker:down
task docker:logs
```

### Building

```bash
task build
# Binary will be in ./bin/integration
```

### Testing

```bash
# Run all tests
task test

# Run with coverage (generates coverage.html)
task test:coverage

# Run with race detector
task test:race
```

## Monitoring

The service logs:
- Articles found per city
- Articles posted successfully
- Articles skipped (duplicates or non-crime)
- Errors during processing

For production, consider adding:
- Prometheus metrics
- Structured logging (JSON)
- Health check endpoint
- Alerting on errors

## Troubleshooting

### Elasticsearch Connection Issues

- Verify ES is running: `curl http://localhost:9200`
- Check credentials in config
- Ensure network connectivity

### Drupal API Errors

- Verify JSON:API is enabled
- Check OAuth token is valid
- Verify content type and group UUIDs exist
- Check Drupal logs for detailed errors

### Redis Connection Issues

- Verify Redis is running: `redis-cli ping`
- Check connection string format
- Ensure Redis is accessible from the service

## Future Enhancements

- [ ] ML-based crime classification (using OpenAI API or local model)
- [ ] Webhook notifications for posted articles
- [ ] Health check HTTP endpoint
- [ ] Prometheus metrics
- [ ] Support for multiple content types
- [ ] Retry logic with exponential backoff
- [ ] Article content enrichment
- [ ] Scheduled posting (delay between posts)

## License

MIT

## Contributing

Contributions welcome! Please open an issue or submit a pull request.
