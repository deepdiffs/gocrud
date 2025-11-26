 # gocrud

 A simple CRUD REST API in Go, backed by in-memory storage.

 ## Prerequisites

 * Go 1.21+ (for local build)
 * Docker (for container builds)

 ## Local Build & Run

```bash
export API_KEYS="<your-api-key>"
go build -o gocrud cmd/server/main.go
./gocrud
```

 The server listens on port 9090 by default.

 ## Configuration

 Environment variables:

* `HTTP_ADDR` – HTTP listen address (default: `:9090`)
* `API_KEYS` – comma-separated list of valid API keys (required)
* `STORAGE_BACKEND` – storage backend (default: `memory`)

 ## Docker

 Build a multi-architecture Docker image:

 ```bash
 # ensure buildx is initialized
 docker buildx create --use

 docker buildx build \
   --platform linux/amd64,linux/arm64 \
   --tag gocrud:latest \
   --load \
   .
 ```

Run the container:

```bash
docker run --rm -p 9090:9090 \
  -e API_KEYS="<your-api-key>" \
  gocrud:latest
```

### Docker Compose

Use Docker Compose to run gocrud:

```bash
# Create a .env file containing your API keys (or export API_KEYS in your shell):
echo 'API_KEYS="<your-api-key>"' > .env
docker compose up --build
```

The HTTP API will be available at http://localhost:9090.

## Authentication

All API endpoints require authentication via the `X-API-Key` header. The API key must be one of the keys specified in the `API_KEYS` environment variable.

Example request:
```bash
curl -H "X-API-Key: your-api-key-here" http://localhost:9090/items
```

## API Endpoints

 | Method | Path          | Description                         |
 | ------ | ------------- | ----------------------------------- |
 | POST   | `/items`      | Create a new item                   |
 | GET    | `/items`      | List all items (filter by type)     |
 | GET    | `/items/{id}` | Retrieve an item by ID              |
 | PUT    | `/items/{id}` | Update an item                      |
 | DELETE | `/items/{id}` | Delete an item                      |

All endpoints require the `X-API-Key` header for authentication.

## Integration Tests

An end-to-end integration test suite is provided in `integration_test.go`. It starts the HTTP server and exercises all CRUD operations against the in-memory storage backend.

```bash
go test -timeout 1m
```