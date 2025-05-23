syntax=docker/dockerfile:1

ARG TARGETOS
ARG TARGETARCH

FROM golang:1.21-alpine AS builder
WORKDIR /app

# cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# build application
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /gocrud main.go

FROM scratch
COPY --from=builder /gocrud /gocrud

EXPOSE 9090
ENTRYPOINT ["/gocrud"]