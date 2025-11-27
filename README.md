 # gocrud

 A simple CRUD REST API in Go with pluggable storage backends (in-memory or Firestore).

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

* `HTTP_ADDR` – HTTP listen address (default: `:9090`; on Cloud Run the service now respects `PORT` if `HTTP_ADDR` is unset)
* `API_KEYS` – comma-separated list of valid API keys (required)
* `STORAGE_BACKEND` – storage backend (default: `memory`, set to `firestore` to use Firestore)
* `FIRESTORE_COLLECTION` – override the default collection name when using the Firestore backend (default: `items`)
* `GOOGLE_CLOUD_PROJECT` – Firestore project ID; if unset, the server now auto-detects the project from default credentials or the Cloud Run metadata server

When running on Cloud Run with the Firestore backend:

* Deploy with `STORAGE_BACKEND=firestore`. `GOOGLE_CLOUD_PROJECT` is optional because the service now falls back to the Cloud Run metadata server.
* Ensure the service account has Firestore access (e.g., `roles/datastore.user`) and that the Firestore API is enabled in the project.

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
