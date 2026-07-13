.PHONY: build build-prod

build:
	docker run --rm --platform linux/amd64 \
	  -v $(shell pwd):/app -w /app \
	  golang:1.24 \
	  sh -c "go mod tidy && go mod vendor && sed -i 's/^go 1\.24.*/go 1.23.0/; /^toolchain /d' go.mod"
	docker run --rm --platform linux/amd64 \
	  -v $(shell pwd):/app -w /app \
	  tinygo/tinygo:0.35.0 \
	  tinygo build -target=wasi -mod=vendor -o voting_app.wasm .

build-prod:
	docker run --rm --platform linux/amd64 \
	  -v $(shell pwd):/app -w /app \
	  golang:1.24 \
	  sh -c "go mod tidy && go mod vendor && sed -i 's/^go 1\.24.*/go 1.23.0/; /^toolchain /d' go.mod"
	docker run --rm --platform linux/amd64 \
	  -v $(shell pwd):/app -w /app \
	  tinygo/tinygo:0.35.0 \
	  tinygo build -opt=s -no-debug -target=wasi -mod=vendor -o voting_app.wasm .
