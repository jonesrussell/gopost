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
- `APP_DEBUG` - Enable debug mode (`true`, `1`, `yes` for debug, anything else for production)

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

### Application Settings

- `debug`: Enable debug mode (default: `false`)
  - `true`: Development logger (human-readable, colorized)
  - `false`: Production logger (JSON format, optimized)
  - Can be overridden with `APP_DEBUG` environment variable

### Service Settings

- `check_interval`: How often to check for new articles (e.g., "5m", "1h")
- `rate_limit_rps`: Maximum requests per second to Drupal
- `lookback_hours`: How many hours back to search in Elasticsearch (0 = no date filter)
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

## Logging

The service uses structured logging powered by [zap](https://github.com/uber-go/zap) for high-performance, structured logging.

### Log Format

The log format depends on the `debug` configuration setting:

**Development Mode (`debug: true`):**
- Human-readable, colorized output with pretty formatting
- Color-coded log levels (DEBUG=magenta, INFO=blue, WARN=yellow, ERROR=red)
- Stack traces for warnings and errors (not for debug/info to reduce noise)
- Clean ISO8601 timestamp format
- Short caller format (file:line) for easy debugging
- Pretty-printed structured fields
- Example output:
  ```
  2025-12-09T19:30:00.000Z	INFO	service.go:123	Starting article sync	{"service": "gopost", "version": "1.0.0"}
  ```

**Production Mode (`debug: false`):**
- JSON-formatted output
- Optimized for performance
- Stack traces only for errors and above
- Example: `{"level":"info","ts":1702143000.0,"caller":"service.go:123","msg":"Starting article sync","service":"gopost","version":"1.0.0"}`

### Configuration

Enable debug mode in `config.yml`:

```yaml
# Application debug mode
# When true: uses development logger (human-readable, colorized output)
# When false: uses production logger (JSON format, optimized for performance)
# Can be overridden with APP_DEBUG environment variable
debug: true
```

Or via environment variable:

```bash
export APP_DEBUG=true
```

The `APP_DEBUG` environment variable accepts:
- `true`, `1`, `yes` (case-insensitive) → enables debug mode
- Any other value → disables debug mode (production)

### Log Levels

The service uses the following log levels:

- **Debug**: Detailed information for troubleshooting (queries, processing steps, cache operations)
- **Info**: General informational messages (service start/stop, articles found/posted, sync completion)
- **Warn**: Non-critical issues (failed to mark article as posted, TLS verification disabled)
- **Error**: Failures requiring attention (API errors, connection failures, processing errors)

### Common Log Fields

The service uses consistent field naming (snake_case) across all logs:

- `article_id` - Unique identifier for an article
- `city` - City name being processed
- `index_name` - Elasticsearch index name
- `error` - Error details (when using Error() field helper)
- `duration` - Operation duration (time.Duration)
- `query_duration` - Elasticsearch query execution time
- `post_duration` - Time to post article to Drupal
- `request_duration` - HTTP request duration
- `status_code` - HTTP response status code
- `service` - Service name (always "gopost")
- `version` - Application version

### Example Log Entries

**Info level - Article posted:**
```json
{
  "level": "info",
  "msg": "Posted article",
  "title": "Police arrest suspect",
  "city": "sudbury_com",
  "article_id": "abc123",
  "url": "https://example.com/article",
  "post_duration": "150ms",
  "article_processing_duration": "200ms"
}
```

**Debug level - Cache check:**
```json
{
  "level": "debug",
  "msg": "Checking if article was posted",
  "article_id": "abc123",
  "redis_key": "posted:article:abc123"
}
```

**Error level - API failure:**
```json
{
  "level": "error",
  "msg": "Drupal API error",
  "endpoint": "https://drupal.site/jsonapi/node/article",
  "article_title": "Police arrest suspect",
  "status_code": 400,
  "error_detail": "Field group_id is required",
  "request_duration": "120ms"
}
```

### Logging Best Practices

1. **Use structured fields** instead of string concatenation:
   ```go
   // Good
   logger.Info("Article posted",
       logger.String("article_id", id),
       logger.String("title", title),
   )
   
   // Avoid
   logger.Info(fmt.Sprintf("Article %s titled '%s' posted", id, title))
   ```

2. **Use appropriate log levels**:
   - Debug: Detailed troubleshooting info
   - Info: Important business events
   - Warn: Non-critical issues
   - Error: Failures requiring attention

3. **Include context** using the `With()` method:
   ```go
   requestLogger := logger.With(
       logger.String("request_id", "abc-123"),
       logger.String("user_id", "user-456"),
   )
   ```

4. **Add duration fields** for performance monitoring:
   ```go
   start := time.Now()
   // ... do work ...
   logger.Info("Operation completed",
       logger.Duration("duration", time.Since(start)),
   )
   ```

## Monitoring

The service logs:
- Articles found per city
- Articles posted successfully
- Articles skipped (duplicates or non-crime)
- Errors during processing
- Performance metrics (durations for all operations)

For production, consider adding:
- Prometheus metrics
- Health check endpoint
- Alerting on errors
- Log aggregation (ELK, Loki, etc.)

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
