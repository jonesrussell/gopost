# CLAUDE.md - AI Assistant Guide for GoPost Integration Service

This document provides a comprehensive guide for AI assistants working with the GoPost Integration Service codebase. It explains the architecture, conventions, and development workflows to help AI assistants make informed decisions when modifying or extending the code.

## Table of Contents

1. [Project Overview](#project-overview)
2. [Architecture & Components](#architecture--components)
3. [Directory Structure](#directory-structure)
4. [Key Conventions](#key-conventions)
5. [Development Workflow](#development-workflow)
6. [Testing Strategy](#testing-strategy)
7. [Common Tasks](#common-tasks)
8. [Important Guidelines for AI Assistants](#important-guidelines-for-ai-assistants)

---

## Project Overview

**GoPost** is a Go-based integration service that bridges Elasticsearch and Drupal 11, specifically designed for filtering and posting crime-related news articles.

### Purpose
- Query Elasticsearch for articles crawled from news sites
- Filter crime-related content using keyword matching
- Post filtered articles to Drupal via JSON:API
- Prevent duplicate posts using Redis-based deduplication
- Support multi-city configurations with rate limiting

### Tech Stack
- **Language**: Go 1.25+
- **Build Tool**: Task (taskfile.dev)
- **External Services**:
  - Elasticsearch 8.x (article storage)
  - Drupal 11 (content management via JSON:API)
  - Redis 6.x+ (deduplication tracking)

### Key Features
- Crime article filtering using configurable keywords
- Drupal JSON:API integration with group support
- Redis-based deduplication (1-year TTL)
- Rate limiting to prevent API overload
- Graceful shutdown handling (SIGTERM/SIGINT)
- Multi-city support with separate indexes
- Structured logging with zap
- Environment variable overrides

---

## Architecture & Components

### System Flow

```
┌─────────────────┐
│  Go Crawler     │ (external system)
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

### Core Components

#### 1. **main.go** (`/main.go`)
- Entry point for the application
- Handles:
  - Configuration loading
  - Logger initialization based on debug mode
  - Service creation and lifecycle management
  - Graceful shutdown signals (SIGTERM/SIGINT)
  - Version info (set via ldflags at build time)

#### 2. **Config Package** (`internal/config/`)
- **Purpose**: Configuration management with YAML and environment variable support
- **Key Files**:
  - `config.go`: Configuration structures and loading logic
  - `config_test.go`: Configuration tests
- **Environment Variables**:
  - `ES_URL`: Elasticsearch URL
  - `DRUPAL_URL`: Drupal site URL
  - `DRUPAL_USERNAME`: REST API username
  - `DRUPAL_TOKEN`: API authentication token
  - `DRUPAL_AUTH_METHOD`: AUTH-METHOD header (miniOrange)
  - `REDIS_URL`: Redis connection string
  - `APP_DEBUG`: Debug mode (true/1/yes for debug, otherwise production)
- **Defaults**:
  - Check interval: 5 minutes
  - Rate limit: 10 RPS
  - Lookback hours: 0 (no date filter by default)
  - Content type: `node--article`
  - Group type: `group--crime_news`

#### 3. **Logger Package** (`internal/logger/`)
- **Purpose**: Structured logging wrapper around uber/zap
- **Key Files**:
  - `logger.go`: Logger interface and zap implementation
  - `fields.go`: Field helper functions
  - `example_test.go`: Usage examples
- **Modes**:
  - **Debug Mode** (`debug: true`):
    - Human-readable, colorized console output
    - Color-coded log levels (DEBUG=magenta, INFO=blue, WARN=yellow, ERROR=red)
    - ISO8601 timestamps
    - Stack traces for warnings and errors
    - Short caller format (file:line)
  - **Production Mode** (`debug: false`):
    - JSON-formatted output
    - Performance-optimized
    - Stack traces only for errors
- **Field Naming Convention**: Always use snake_case (e.g., `article_id`, `status_code`, `request_duration`)

#### 4. **Drupal Client Package** (`internal/drupal/`)
- **Purpose**: Drupal JSON:API client for posting articles
- **Key File**: `client.go`
- **Features**:
  - JSON:API article posting
  - CSRF token fetching
  - Multiple authentication methods:
    - API-KEY header with base64(username:api-key)
    - Authorization header with Basic auth
    - AUTH-METHOD header (miniOrange support)
  - TLS verification skip option (development only)
  - Comprehensive error logging with validation details
  - Support for group relationships
  - Field URL handling
- **Important Structures**:
  - `ArticleRequest`: Request parameters
  - `DrupalArticle`: JSON:API formatted article
  - `GroupReference`: Group relationship structure
  - `DrupalResponse`: API response with error handling

#### 5. **Deduplication Package** (`internal/dedup/`)
- **Purpose**: Track posted articles to prevent duplicates
- **Key File**: `tracker.go`
- **Redis Keys**: `posted:article:{article_id}`
- **TTL**: 365 days (1 year)
- **Methods**:
  - `HasPosted(ctx, articleID)`: Check if article was posted
  - `MarkPosted(ctx, articleID)`: Mark article as posted
  - `Clear(ctx, articleID)`: Remove from posted cache

#### 6. **Integration Service Package** (`internal/integration/`)
- **Purpose**: Core business logic orchestrating all components
- **Key File**: `service.go`
- **Responsibilities**:
  - Elasticsearch query construction
  - Crime article filtering
  - City processing
  - Rate limiting coordination
  - Periodic sync scheduling
- **Key Methods**:
  - `NewService()`: Initialize service with all dependencies
  - `FindCrimeArticles()`: Query ES for crime-related articles
  - `ProcessCity()`: Process articles for a single city
  - `Run()`: Main loop with ticker-based scheduling
  - `runOnce()`: Single sync iteration
  - `isCrimeRelated()`: Keyword-based filtering

#### 7. **Utilities** (`cmd/`)
- **getnode** (`cmd/getnode/`): Debug utility to fetch and display Drupal nodes

---

## Directory Structure

```
gopost/
├── cmd/                      # Command-line utilities
│   └── getnode/             # Debug tool for fetching Drupal nodes
│       └── main.go
├── internal/                # Internal packages (not importable externally)
│   ├── config/             # Configuration management
│   │   ├── config.go
│   │   └── config_test.go
│   ├── dedup/              # Redis-based deduplication
│   │   └── tracker.go
│   ├── drupal/             # Drupal JSON:API client
│   │   └── client.go
│   ├── integration/        # Core integration service
│   │   └── service.go
│   └── logger/             # Structured logging
│       ├── logger.go
│       ├── fields.go
│       ├── logger_test.go
│       └── example_test.go
├── .devcontainer/          # VS Code devcontainer configuration
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Dependency checksums
├── Taskfile.yml            # Task runner configuration
├── Dockerfile              # Production container image
├── docker-compose.yml      # Multi-service orchestration
├── config.yml.example      # Example configuration
├── .golangci.yml          # Linter configuration
├── .gitignore             # Git ignore patterns
└── README.md              # User-facing documentation
```

### Package Import Path
- Module: `github.com/gopost/integration`
- Internal packages: `github.com/gopost/integration/internal/{package}`

---

## Key Conventions

### 1. Code Style

#### Go Standards
- Follow standard Go formatting (use `gofmt` / `goimports`)
- Use descriptive variable names (no single-letter vars except loop indexes)
- Keep functions focused and small
- Prefer composition over inheritance
- Use interfaces for abstraction

#### Error Handling
- Always wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Log errors at the appropriate level before returning
- Include relevant context in error messages
- Don't ignore errors (use `_ = err` if intentional with comment)

#### Context Usage
- Always pass `context.Context` as the first parameter
- Use context for cancellation and timeouts
- Respect context cancellation in long-running operations

### 2. Logging Conventions

#### Log Levels
- **Debug**: Detailed troubleshooting info (queries, cache checks, payloads)
- **Info**: Important business events (articles found, posted, sync completion)
- **Warn**: Non-critical issues (failed cache updates, TLS verification disabled)
- **Error**: Failures requiring attention (API errors, connection failures)

#### Field Naming
- **Always use snake_case** for field names
- Common fields:
  - `article_id`: Article identifier
  - `city`: City being processed
  - `index_name`: Elasticsearch index
  - `status_code`: HTTP status
  - `duration`, `query_duration`, `post_duration`: Time measurements
  - `error`: Error details (use `logger.Error(err)` helper)

#### Structured Logging Pattern
```go
logger.Info("Operation completed",
    logger.String("article_id", id),
    logger.Duration("duration", time.Since(start)),
    logger.Int("count", len(items)),
)
```

#### Context-Aware Logging
```go
// Add persistent fields to logger
methodLogger := logger.With(
    logger.String("method", "PostArticle"),
    logger.String("city", cityName),
)
```

### 3. Configuration

#### YAML Structure
- Use snake_case for YAML keys
- Group related settings under sections
- Include comments explaining each option
- Provide sensible defaults

#### Environment Variable Overrides
- All critical settings should support env var overrides
- Use uppercase with underscores (e.g., `DRUPAL_TOKEN`)
- Document in README and config.yml.example

### 4. Elasticsearch Field Naming

**Important**: The Elasticsearch schema uses specific field names that differ from typical conventions:

- `body` (not `content`) - Article content
- `canonical_url` (not `url`) - Article URL
- `published_date` (not `published_at`) - Publication timestamp

Always map these correctly in the `Article` struct.

### 5. Drupal JSON:API

#### Content Type Format
- Format: `node--{content_type}` (e.g., `node--article`)
- Group type: `group--{group_type}` (e.g., `group--crime_news`)

#### Field Naming
- Use `field_` prefix for custom fields
- `field_url`: Link field with URI structure
- `field_group`: Entity reference to groups

#### Relationships
- Groups go in `relationships.field_group.data[]` as an array
- Each relationship needs `type` (group type) and `id` (UUID)
- Never use numeric IDs directly; always use UUIDs

#### Authentication Headers
- `Content-Type: application/vnd.api+json`
- `Accept: application/vnd.api+json`
- `API-KEY: {base64(username:token)}`
- `Authorization: Basic {base64(username:token)}`
- `AUTH-METHOD: {application_id}` (if using miniOrange)
- `X-CSRF-Token: {token}` (for POST requests)

### 6. Testing Conventions

#### File Naming
- Test files: `{package}_test.go`
- Place in same directory as code under test

#### Test Structure
- Use table-driven tests for multiple cases
- Name tests descriptively: `TestFunctionName_Scenario`
- Use t.Run() for subtests
- Always test error cases

#### Example Test Pattern
```go
func TestLoadConfig_ValidFile(t *testing.T) {
    cfg, err := Load("testdata/valid.yml")
    if err != nil {
        t.Fatalf("expected no error, got: %v", err)
    }
    if cfg.Debug != true {
        t.Errorf("expected debug=true, got: %v", cfg.Debug)
    }
}
```

### 7. Linting and Code Quality

#### Running the Linter
```bash
golangci-lint run
```

#### Linter Configuration
The project uses golangci-lint with strict rules configured in `.golangci.yml`. Key settings:

- **Line Length**: Maximum 150 characters
- **Function Length**: Maximum 100 lines, 50 statements
- **Cognitive Complexity**: Maximum 20
- **Cyclomatic Complexity**: Maximum 30

#### Intentional Pattern Exceptions

Some linter warnings are intentionally suppressed for valid reasons:

1. **Canonical Headers** (internal/drupal/client.go):
   - `API-KEY`, `AUTH-METHOD`, `X-CSRF-Token` - Required exact names by Drupal REST API
   - Use `//nolint:canonicalheader` with explanation

2. **Magic Numbers**:
   - HTTP status codes (400, 404, etc.) - Standard HTTP conventions
   - Timeout values (30 seconds, 5 minutes) - Common defaults
   - Use constants for application-specific numbers

3. **TLS Skip Verify** (G402):
   - Allowed in development mode only
   - Must log warning when enabled
   - Never use in production

4. **Debug Utilities** (cmd/getnode):
   - `fmt.Println` allowed for debug output
   - These are development tools, not production code

5. **Test Packages**:
   - White-box testing (same package) allowed for config and logger tests
   - Use black-box testing (`_test` package) for other packages

#### Using nolint Directives

When suppressing linter warnings, always provide explanation:

```go
//nolint:canonicalheader // Drupal REST API requires exact header name
req.Header.Set("API-KEY", apiKeyValue)

//nolint:gosec // G402: TLS skip verify intentional for development
client.Transport = &http.Transport{
    TLSClientConfig: &tls.Config{
        InsecureSkipVerify: true,
    },
}
```

#### Common Linter Issues and Fixes

1. **Error Handling**:
   ```go
   // Bad
   if err != nil && err != context.Canceled {

   // Good
   if err != nil && !errors.Is(err, context.Canceled) {
   ```

2. **Shadow Variables**:
   ```go
   // Bad
   if err := someFunc(); err != nil {
       err := anotherFunc() // shadows outer err
   }

   // Good
   if err := someFunc(); err != nil {
       newErr := anotherFunc()
   }
   ```

3. **Defer with os.Exit**:
   ```go
   // Bad
   defer logger.Sync()
   // ... later ...
   os.Exit(1) // defer won't run

   // Good
   _ = logger.Sync()
   os.Exit(1)
   ```

4. **Interface{} vs Any**:
   ```go
   // Old (pre-Go 1.18)
   func Process(data map[string]interface{}) error

   // Modern (Go 1.18+)
   func Process(data map[string]any) error
   ```

5. **HTTP Methods and Bodies**:
   ```go
   // Bad
   req, err := http.NewRequest("GET", url, nil)

   // Good
   req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
   ```

---

## Development Workflow

### 1. Local Development Setup

```bash
# Install dependencies
task deps

# Run tests
task test

# Run with coverage
task test:coverage

# Format code
task fmt

# Run linter
task lint

# Run all checks
task check

# Build binary
task build

# Run locally
task run
```

### 2. Docker Development

```bash
# Build image
task docker:build

# Start all services (ES, Redis, Drupal, integration)
task docker:up

# View logs
task docker:logs

# Stop services
task docker:down

# Restart services
task docker:restart
```

### 3. Build Process

#### Local Build
```bash
task build
# Output: ./bin/integration
```

#### Docker Build
```bash
docker build -t gopost-integration .
```

#### Versioned Build
```bash
go build -ldflags "-X main.version=v1.2.3" -o bin/integration main.go
```

### 4. Configuration Management

1. Copy example config: `cp config.yml.example config.yml`
2. Edit `config.yml` with your settings
3. Override with env vars as needed:
   ```bash
   export DRUPAL_TOKEN="your-token"
   export APP_DEBUG=true
   task run
   ```

### 5. Git Workflow

- **Main Branch**: `main` (or as configured)
- **Feature Branches**: `claude/claude-md-{session-id}-{unique-id}`
- **Commits**: Clear, descriptive messages
- **Push**: Always to feature branch with `git push -u origin {branch-name}`

---

## Testing Strategy

### Unit Tests
- Test individual functions in isolation
- Mock external dependencies (ES, Drupal, Redis)
- Focus on business logic correctness
- Target: 80%+ code coverage

### Integration Tests
- Test component interactions
- Use real Redis (testcontainers if possible)
- Mock only external services (ES, Drupal)

### Running Tests

```bash
# All tests
task test

# With coverage
task test:coverage
# Opens coverage.html in browser

# With race detector
task test:race

# Specific package
go test -v ./internal/config

# Specific test
go test -v -run TestLoadConfig ./internal/config
```

### Test Data
- Store test fixtures in `testdata/` directories
- Use meaningful file names
- Document any special test requirements

---

## Common Tasks

### Adding a New City

1. Edit `config.yml`:
   ```yaml
   cities:
     - name: "new_city_com"
       index: "new_city_com_articles"
       group_id: "uuid-from-drupal"
   ```

2. Ensure Elasticsearch index exists with correct schema
3. Create corresponding group in Drupal
4. Restart service to pick up changes

### Adding New Crime Keywords

1. Edit `config.yml`:
   ```yaml
   service:
     crime_keywords:
       - "existing keyword"
       - "new keyword"
   ```

2. Test with debug mode to verify matches:
   ```yaml
   debug: true
   ```

### Debugging Elasticsearch Queries

1. Enable debug mode in `config.yml`
2. Check logs for "Elasticsearch query" entries
3. Review query structure and results
4. Use `curl` or Kibana to test queries directly

### Debugging Drupal API Calls

1. Enable debug mode
2. Look for "Article payload prepared" log entries
3. Check HTTP status codes and error messages
4. Use `cmd/getnode` utility to verify API access:
   ```bash
   go run cmd/getnode/main.go
   ```

### Adding a New Field to Articles

1. Update `Article` struct in `internal/integration/service.go`
2. Update Elasticsearch query field mappings
3. Update `DrupalArticle` struct in `internal/drupal/client.go`
4. Update `PostArticle` method to include new field
5. Add tests for new field handling

### Changing Rate Limits

Edit `config.yml`:
```yaml
service:
  rate_limit_rps: 20  # Increase from 10 to 20 RPS
```

---

## Important Guidelines for AI Assistants

### When Making Changes

1. **Always Read Before Editing**
   - Use the Read tool to view files before modifying
   - Understand the context and existing patterns
   - Preserve existing code style and conventions

2. **Follow Go Best Practices**
   - Run `task fmt` after code changes
   - Run `task vet` to check for issues
   - Run `task test` to ensure tests pass
   - Use `task check` for comprehensive validation

3. **Logging is Critical**
   - Add appropriate logging for new operations
   - Use structured fields (snake_case)
   - Include timing information (durations)
   - Log errors with full context before returning

4. **Error Handling**
   - Never ignore errors
   - Wrap errors with context
   - Log before returning errors
   - Consider partial failures (continue processing other items)

5. **Testing Requirements**
   - Add tests for new functionality
   - Update existing tests if behavior changes
   - Ensure tests are deterministic
   - Use table-driven tests for multiple cases

6. **Configuration Changes**
   - Update `config.yml.example` if adding new settings
   - Add environment variable support for new settings
   - Set sensible defaults
   - Document in README.md

7. **Backward Compatibility**
   - Don't break existing configuration files
   - Provide migration path for breaking changes
   - Use defaults for new optional fields
   - Version the API if making incompatible changes

### Common Pitfalls to Avoid

1. **Field Name Mismatches**
   - ES uses `body`, `canonical_url`, `published_date`
   - Drupal uses `field_` prefix for custom fields
   - Logger uses `snake_case` everywhere

2. **UUID vs Numeric IDs**
   - Always use UUIDs for Drupal group references
   - Never use numeric IDs in JSON:API calls
   - Document UUID requirements clearly

3. **Array vs Object in Relationships**
   - `field_group.data` is an array `[]GroupReference`
   - Even for single items, use array format

4. **Authentication Headers**
   - Always include all required headers
   - Base64 encode credentials properly
   - Include CSRF token for mutations

5. **Context Cancellation**
   - Always check context in long-running loops
   - Respect context deadlines
   - Use context-aware methods (e.g., `client.Do(req.WithContext(ctx))`)

6. **Rate Limiting**
   - Never bypass rate limiter
   - Wait before operations, not after
   - Consider rate limits when adding new API calls

### Documentation Updates

When modifying the codebase, update:
1. This CLAUDE.md file for architectural changes
2. README.md for user-facing changes
3. Code comments for complex logic
4. config.yml.example for new settings
5. Inline godoc comments for exported functions

### Security Considerations

1. **Never Hardcode Secrets**
   - Use environment variables
   - Use configuration files (excluded from git)
   - Document required secrets in README

2. **TLS Verification**
   - Only skip TLS verification in development
   - Log warnings when TLS verification is disabled
   - Never skip in production

3. **Input Validation**
   - Validate configuration on load
   - Check required fields
   - Validate URLs and formats

4. **Credential Handling**
   - Use base64 encoding correctly
   - Don't log credentials
   - Clear sensitive data when not needed

### Performance Considerations

1. **Efficient ES Queries**
   - Use date filters when possible
   - Limit result size appropriately
   - Use appropriate index mappings

2. **Rate Limiting**
   - Respect configured limits
   - Use burst capacity wisely
   - Don't DOS Drupal

3. **Logging Overhead**
   - Use debug level for verbose logs
   - Avoid logging large payloads in production
   - Use sampling for high-frequency events

4. **Connection Pooling**
   - Reuse HTTP clients
   - Configure timeouts appropriately
   - Close resources properly

### Git Operations

1. **Branch Naming**
   - Must start with `claude/`
   - Must end with session ID
   - Format: `claude/claude-md-{session}-{unique-id}`

2. **Commit Messages**
   - Clear, descriptive messages
   - Reference related issues
   - Explain "why" not just "what"

3. **Pushing Changes**
   - Always use: `git push -u origin {branch-name}`
   - Retry on network failures (up to 4 times, exponential backoff)
   - Never force push to main/master

---

## Additional Resources

### External Documentation
- [Drupal JSON:API](https://www.drupal.org/docs/core-modules-and-themes/core-modules/jsonapi-module)
- [Elasticsearch Go Client](https://github.com/elastic/go-elasticsearch)
- [uber/zap Logger](https://github.com/uber-go/zap)
- [Task Runner](https://taskfile.dev/)

### Internal Documentation
- README.md: User-facing documentation
- config.yml.example: Configuration reference
- Code comments: Inline documentation

---

## Version History

- **Initial Version**: Created based on codebase analysis as of 2025-12-09
- Reflects current architecture with Elasticsearch, Drupal, and Redis integration
- Includes logging conventions with zap
- Documents authentication requirements for miniOrange REST API

---

## Questions or Clarifications?

If you encounter scenarios not covered in this guide:
1. Check the README.md for user-facing documentation
2. Review existing code patterns in similar components
3. Examine test files for usage examples
4. Check git commit history for context on past changes
5. Ask for clarification when assumptions are needed

**Remember**: When in doubt, prefer reading existing code patterns over making assumptions. Consistency with the existing codebase is more important than theoretical "best practices."
