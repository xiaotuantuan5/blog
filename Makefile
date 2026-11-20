BIN      ?= bin/blog
GOOS     ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)
LDFLAGS  := -s -w

.PHONY: build web go test check chroma-css clean dev-api dev-web

## build: 构建前端并打包为单个二进制（GOOS/GOARCH 可交叉编译）
build: web go

web: web/node_modules
	cd web && npm run build
	touch web/dist/.gitkeep

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

go:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/blog

## test: 运行后端测试
test:
	go test ./...

## check: 后端测试 + vet + 前端类型检查
check: test
	go vet ./...
	cd web && npm run typecheck

## chroma-css: 重新生成代码高亮样式
chroma-css:
	go run ./tools/chromacss web/src/styles/chroma.css

## dev-api / dev-web: 开发时分别在两个终端运行，然后访问 http://localhost:5173
dev-api:
	go run ./cmd/blog serve -addr 127.0.0.1:8080 -data ./data

dev-web: web/node_modules
	cd web && npm run dev

clean:
	rm -rf bin
	find web/dist -mindepth 1 ! -name .gitkeep -delete
