.PHONY: test test-go test-web test-ml dev-api dev-web build

test: test-go test-ml test-web

test-go:
	cd backend && go test ./...

test-ml:
	python3 -m unittest discover -s ml/tests -v

test-web:
	cd frontend && npm ci && npm run check && npm test -- --run && npm run build

dev-api:
	cd backend && go run ./cmd/server

dev-web:
	cd frontend && npm install && npm run dev

build:
	cd backend && go build ./cmd/server
	cd frontend && npm ci && npm run build

