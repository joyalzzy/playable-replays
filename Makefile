.PHONY: test test-go test-web test-ml test-detector-evaluation test-pipeline dev-api dev-web dev-telemetry-demo build

test: test-go test-ml test-pipeline test-web

test-go:
	cd backend && go test ./...

test-ml:
	python3 -m unittest discover -s ml/tests -v
	python3 -m ml.evaluate.detector

test-detector-evaluation:
	python3 -m ml.evaluate.detector

test-pipeline:
	python3 scripts/test_telemetry_scenario_pipeline.py

test-web:
	cd frontend && npm ci && npm run check && npm test -- --run && npm run build

dev-api:
	cd backend && go run ./cmd/server

dev-web:
	cd frontend && npm install && npm run dev

dev-telemetry-demo:
	cd backend && go run ./cmd/telemetry-collector --input ../fixtures/telemetry-demo.json --rate 4

build:
	cd backend && go build ./cmd/server
	cd frontend && npm ci && npm run build

