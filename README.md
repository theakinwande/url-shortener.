# High-Performance URL Shortener

A production-grade URL shortening service built in Go, demonstrating high-performance HTTP handling, caching strategies, and clean architecture.

## 🏗️ Architecture

```
Client → API Layer (Gin) → Redis Cache → PostgreSQL
```

### The Flow Explained

1. **Shorten Request**: Client sends URL → Validate → Generate short code → Store in DB → Cache in Redis → Return
2. **Redirect Request**: Client hits short URL → Check Redis → If miss, check DB → Cache result → 302 Redirect

## 🔐 Security Features

- **API Key Authentication**: All mutation endpoints require valid API keys
- **Rate Limiting**: Token bucket algorithm prevents abuse (60 req/min default)
- **Input Validation**: Strict URL validation, SQL injection prevention via parameterized queries
- **Secure Headers**: CORS, content-type validation
- **Password Hashing**: API keys are hashed with SHA-256 before storage

## 🚀 Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose

### Run the Services

```bash
# Start PostgreSQL and Redis
docker-compose up -d

# Run the Go server
go run cmd/server/main.go
```

### Environment Variables

| Variable       | Default                | Description                  |
| -------------- | ---------------------- | ---------------------------- |
| `PORT`         | 8080                   | Server port                  |
| `DATABASE_URL` | postgres://...         | PostgreSQL connection string |
| `REDIS_URL`    | redis://localhost:6379 | Redis connection string      |
| `RATE_LIMIT`   | 60                     | Requests per minute          |

## 📚 API Reference

### Create Short URL

```bash
curl -X POST http://localhost:8080/api/shorten \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{"url": "https://github.com", "custom_alias": "gh"}'
```

### Redirect

```bash
curl -I http://localhost:8080/gh
# Returns 302 redirect
```

### Get Stats

```bash
curl http://localhost:8080/api/stats/gh \
  -H "X-API-Key: your-api-key"
```

### Delete URL

```bash
curl -X DELETE http://localhost:8080/api/gh \
  -H "X-API-Key: your-api-key"
```

## 📁 Project Structure

```
short/
├── cmd/server/main.go       # Entry point - starts the server
├── internal/
│   ├── config/              # Environment configuration
│   ├── database/            # DB & Redis connections
│   ├── handler/             # HTTP request handlers
│   ├── middleware/          # Auth & rate limiting
│   ├── models/              # Data structures
│   ├── repository/          # Data access layer
│   └── service/             # Business logic
├── migrations/              # SQL schema files
├── docker-compose.yml       # Container orchestration
└── Dockerfile              # Production build
```

## 🧠 Key Concepts (Learning Notes)

### Why This Architecture?

1. **Repository Pattern**: Separates data access from business logic. If you switch from PostgreSQL to MongoDB, only the repository changes.

2. **Service Layer**: Contains business rules. "Should this URL be shortened?" logic lives here, not in handlers.

3. **Middleware Chain**: Each request passes through: Rate Limit → API Key Check → Handler. Clean separation of concerns.

### Why Redis + PostgreSQL?

- **Redis**: Lightning fast (in-memory), perfect for hot data (frequently accessed URLs)
- **PostgreSQL**: Durable, ACID-compliant, stores everything permanently
- **Together**: 99% of redirects hit cache, DB only for cold data

### Rate Limiting (Token Bucket)

Imagine a bucket that holds 60 tokens. Each request consumes 1 token. Tokens refill at 1/second. If bucket is empty → 429 Too Many Requests.

This is implemented in Redis using `INCR` with expiration.
