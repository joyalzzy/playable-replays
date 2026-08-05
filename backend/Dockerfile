FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY backend/go.mod ./
COPY backend/cmd ./cmd
COPY backend/internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.21
RUN adduser -D -H -u 10001 app
WORKDIR /app
RUN mkdir -p /app/data && chown app:app /app/data
COPY --from=build /out/server /app/server
COPY fixtures /app/fixtures
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]
