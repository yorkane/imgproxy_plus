FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o imgproxy_plus .

FROM ghcr.io/imgproxy/imgproxy:latest
USER root
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /build/imgproxy_plus /usr/local/bin/imgproxy_plus
COPY html/ /usr/local/bin/html/
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/imgproxy_plus
RUN mkdir -p /data /mnt/ramdisk
USER imgproxy
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
