# Build context: the repository root (see fly.toml).
FROM golang:1.27 AS build
WORKDIR /src/app
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM litestream/litestream:0.3.13 AS litestream

FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/server /usr/local/bin/server
COPY --from=litestream /usr/local/bin/litestream /usr/local/bin/litestream
COPY app/litestream.yml /etc/litestream.yml
COPY app/entrypoint.sh /usr/local/bin/entrypoint.sh
# words.jsonl comes from git; examples.jsonl is fetched by the deploy job.
COPY dictionary/ /srv/dictionary/
RUN chmod +x /usr/local/bin/entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
