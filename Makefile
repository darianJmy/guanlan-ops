.PHONY: build web wire test dev dev-web

AIR_VERSION := v1.63.0

# 国内开发环境默认使用镜像；可通过环境变量或 make 参数覆盖。
export GOPROXY ?= https://goproxy.cn,direct

test:
	go test ./...

.DEFAULT_GOAL := build

dev: dev-web
	go run github.com/air-verse/air@$(AIR_VERSION) -c .air.toml $(if $(CONF),-- --conf "$(CONF)")

dev-web:
	@test -f web/dist/index.html || $(MAKE) web

build: web
	CGO_ENABLED=0 go build -trimpath -o guanlan .

web:
	cd web && npm ci && NODE_OPTIONS="--max-old-space-size=1024" npm run build

wire: dev-web
	go run -mod=mod github.com/google/wire/cmd/wire@v0.7.0 ./internal/app
