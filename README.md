 # gocrud

 A simple CRUD REST API in Go, backed by Redis.

 ## Prerequisites

 * Go 1.21+ (for local build)
 * Docker (for container builds)
 * Redis instance running (default at localhost:6379)

 ## Local Build & Run

```bash
export API_KEYS="<your-api-key>"
go build -o gocrud main.go
./gocrud
```

 The server listens on port 9090 by default and connects to Redis at localhost:6379.

 ## Configuration

 Environment variables:

* `REDIS_ADDR` – Redis address (default: `localhost:6379`)
* `HTTP_ADDR` – HTTP listen address (default: `:9090`)
* `API_KEYS` – comma-separated list of valid API keys (required)

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

Run the container (connecting to host Redis):

 ### Linux

```bash
docker run --rm --network host \
  -e API_KEYS="<your-api-key>" \
  gocrud:latest
```

 ### macOS

```bash
docker run --rm -p 9090:9090 \
  -e API_KEYS="<your-api-key>" \
  -e REDIS_ADDR=host.docker.internal:6379 \
  gocrud:latest
```

 ## API Endpoints

 | Method | Path          | Description                         |
 | ------ | ------------- | ----------------------------------- |
 | POST   | `/items`      | Create a new item                   |
 | GET    | `/items`      | List all items (filter by type)     |
 | GET    | `/items/{id}` | Retrieve an item by ID              |
 | PUT    | `/items/{id}` | Update an item                      |
 | DELETE | `/items/{id}` | Delete an item                      |

## Integration Tests

An end-to-end integration test suite is provided in `integration_test.go`. It starts the HTTP server and exercises all CRUD operations against Redis.
Make sure a Redis instance is running at `localhost:6379`, then run:

```bash
go test -timeout 1m
```