FROM golang:1.27.2-bookworm AS build
WORKDIR /src
COPY hyjs-server/go.mod hyjs-server/go.sum ./
RUN go mod download
COPY hyjs-server/ ./
RUN CGO_ENABLED=1 go build -trimpath -o /out/hyjs-server .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/hyjs-server /app/hyjs-server
COPY hyjs-server/config.example.yaml /app/config.yaml
COPY analysis/tables /app/tables
EXPOSE 18088 19098
ENTRYPOINT ["/app/hyjs-server"]
CMD ["--bind", "0.0.0.0", "--tables", "/app/tables"]
