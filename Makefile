.PHONY: test test-go test-web validate-fixtures dev-api dev-web build build-api build-web

test: test-go test-web validate-fixtures

test-go:
	cd backend && go test ./...

test-web:
	cd frontend && npm ci && npm run check && npm test -- --run && npm run build

validate-fixtures:
	cd backend && go run ./cmd/validate-fixtures -path ../fixtures/moments.json

dev-api:
	cd backend && go run ./cmd/server

dev-web:
	cd frontend && npm install && npm run dev

build: build-api build-web

build-api:
	cd backend && go build -o /tmp/playable-replays-api ./cmd/server

build-web:
	cd frontend && npm ci && npm run build
