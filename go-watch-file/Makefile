# Makefile for File Watch Service

# 变量定义
BINARY_NAME=file-watch
BUILD_DIR=build
MAIN_PATH=cmd/main.go

# 默认目标
.PHONY: all
all: clean build

# 构建应用
.PHONY: build
build: deps
	@echo "构建 $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "构建完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 清理构建文件
.PHONY: clean
clean:
	@echo "清理构建文件..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@echo "清理完成"

# 运行测试
.PHONY: test
test:
	@echo "运行测试..."
	go test -v ./...

# 运行测试并生成覆盖率报告
.PHONY: test-coverage
test-coverage:
	@echo "运行测试并生成覆盖率报告..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 格式化代码
.PHONY: fmt
fmt:
	@echo "格式化代码..."
	go fmt ./...

# 代码检查
.PHONY: lint
lint:
	@echo "代码检查..."
	golangci-lint run

# 安装依赖
.PHONY: deps
deps:
	@echo "安装依赖..."
	go mod download
	go mod tidy

# 运行应用
.PHONY: run
run: build
	@echo "运行应用..."
	./$(BUILD_DIR)/$(BINARY_NAME) -config config.yaml

# 开发模式运行
.PHONY: dev
dev:
	@echo "开发模式运行..."
	go run $(MAIN_PATH) -config config.yaml

# 安装到系统
.PHONY: install
install: build
	@echo "安装到系统..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "安装完成"

# 卸载
.PHONY: uninstall
uninstall:
	@echo "卸载..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "卸载完成"

# 帮助信息
.PHONY: help
help:
	@echo "可用的目标:"
	@echo "  build        - 构建应用"
	@echo "  clean        - 清理构建文件"
	@echo "  test         - 运行测试"
	@echo "  test-coverage- 运行测试并生成覆盖率报告"
	@echo "  fmt          - 格式化代码"
	@echo "  lint         - 代码检查"
	@echo "  deps         - 安装依赖"
	@echo "  run          - 构建并运行应用"
	@echo "  dev          - 开发模式运行"
	@echo "  install      - 安装到系统"
	@echo "  uninstall    - 卸载"
	@echo "  help         - 显示帮助信息"
