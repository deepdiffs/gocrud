ARG TARGETOS=linux
ARG TARGETARCH=amd64

FROM golang:1.24-alpine AS builder
WORKDIR /app

# cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# build application
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /gocrud ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /gocrud /gocrud

EXPOSE 9090
ENTRYPOINT ["/gocrud"]
