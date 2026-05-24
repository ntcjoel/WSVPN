# WSVPN Server Docker Image
# Build: docker build -t wsvpn-server .
# Run:   docker run -d --cap-add=NET_ADMIN --name wsvpn -p 8180:8180 -v $(pwd)/config:/config wsvpn-server

FROM golang:1.24-alpine AS builder

RUN apk add --no-cache gcc musl-dev linux-headers
WORKDIR /build

# Copy module files and download dependencies first (layer caching)
COPY src/go.mod src/go.sum ./
RUN go mod download

# Copy source and build
COPY src/ ./
# Update external dependencies to latest
RUN go get -u github.com/refraction-networking/utls@latest \
    github.com/gorilla/websocket@latest \
    github.com/quic-go/quic-go@latest
RUN go mod tidy
RUN CGO_ENABLED=1 go build -o /wsvpn-server -ldflags="-s -w" ./server

# Runtime image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /wsvpn-server /wsvpn-server
RUN chmod +x /wsvpn-server

# Create directories
RUN mkdir -p /var/log/wsvpn/server /config

EXPOSE 8180
ENTRYPOINT ["/wsvpn-server", "-config", "/config/server.json"]
