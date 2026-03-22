#!/bin/bash

# 交叉编译 ks-prank 为 Windows 可执行文件

set -e

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

# 输出目录
OUTPUT_DIR="$PROJECT_ROOT/build"
mkdir -p "$OUTPUT_DIR"

# 应用名称
APP_NAME="ks-prank"

# 目标平台
GOOS=windows
GOARCH=amd64

echo "========================================="
echo "  编译 $APP_NAME (Windows amd64)"
echo "========================================="

# 交叉编译（CGO_ENABLED=0 纯静态编译，避免交叉编译时的 C 依赖问题）
CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
    -ldflags="-s -w" \
    -o "$OUTPUT_DIR/${APP_NAME}.exe" \
    .

echo "编译成功: $OUTPUT_DIR/${APP_NAME}.exe"

# 复制配置文件到输出目录
if [ -f "$PROJECT_ROOT/config.yaml" ]; then
    cp "$PROJECT_ROOT/config.yaml" "$OUTPUT_DIR/config.yaml"
    echo "已复制配置文件: $OUTPUT_DIR/config.yaml"
fi

echo "========================================="
echo "  构建完成！"
echo "  输出目录: $OUTPUT_DIR"
echo "========================================="
