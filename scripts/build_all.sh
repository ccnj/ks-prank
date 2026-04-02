#!/bin/bash

# 双平台打包脚本：macOS (.app) + Windows (安装包 .exe)
# 依赖：wails, makensis (brew install makensis), sips (macOS 自带)

set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

BUILD_DIR="$PROJECT_ROOT/build"
BIN_DIR="$BUILD_DIR/bin"

echo "========================================="
echo "  ks-prank 双平台打包"
echo "========================================="

# ---- 1. macOS ----
echo ""
echo "[1/3] 编译 macOS..."
wails build -clean
echo "  macOS 产物: $BIN_DIR/ks-prank.app"

# ---- 2. Windows ----
echo ""
echo "[2/3] 编译 Windows..."
wails build -platform windows/amd64 -clean
echo "  Windows 产物: $BIN_DIR/ks-prank.exe"

# ---- 3. Windows 安装包 ----
echo ""
echo "[3/3] 生成 Windows 安装包..."

# 转换图标: PNG → ICO（使用 sips + iconutil 或 ImageMagick）
if [ ! -f "$BUILD_DIR/appicon.ico" ]; then
    if command -v convert &>/dev/null; then
        convert "$BUILD_DIR/appicon.png" -resize 256x256 "$BUILD_DIR/appicon.ico"
    elif command -v sips &>/dev/null; then
        # macOS: sips 不能直接生成 ico，先生成 bmp 再用 NSIS 的默认图标
        echo "  提示: 未找到 ImageMagick，跳过图标转换（使用 NSIS 默认图标）"
        echo "  如需自定义图标，请安装 ImageMagick: brew install imagemagick"
        # 移除 nsi 中的图标引用，用 sed 注释掉
        SKIP_ICON=true
    fi
fi

# 编译安装包
if [ "$SKIP_ICON" = true ]; then
    makensis -DNOICON "$PROJECT_ROOT/scripts/installer.nsi"
else
    makensis "$PROJECT_ROOT/scripts/installer.nsi"
fi

echo "  安装包产物: $BUILD_DIR/ks-prank-setup.exe"

echo ""
echo "========================================="
echo "  打包完成！产物："
echo "  macOS:   $BIN_DIR/ks-prank.app"
echo "  Windows: $BIN_DIR/ks-prank.exe"
echo "  安装包:  $BUILD_DIR/ks-prank-setup.exe"
echo "========================================="
