# 定义目标平台和架构
PLATFORMS := linux/amd64 darwin/amd64 windows/amd64

# 定义Go程序的入口文件
MAIN_FILE := main.go

# 定义输出目录
OUTPUT_DIR := bin

# 定义编译目标
.PHONY: build
build: $(PLATFORMS)

# 根据目标平台和架构生成可执行文件
$(PLATFORMS):
    # 解析目标平台和架构
    GOOS     ?= $(shell $(GO) env GOHOSTOS)
    GOARCH   ?= $(shell $(GO) env GOHOSTARCH)

    # 生成输出文件名
    $(eval OUTPUT_FILE=$(OUTPUT_DIR)/$(notdir $(basename $(MAIN_FILE)))_$(GOOS)_$(GOARCH))

    # 执行编译命令
    @echo "Building $(OUTPUT_FILE)"
    GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(OUTPUT_FILE) $(MAIN_FILE)

# 定义清理目标
.PHONY: clean
clean:
    rm -rf $(OUTPUT_DIR)