#!/bin/sh
#
# i18n.sh - go-i18n 辅助脚本
#
# 用法：
#   1.填写active.en.yaml
#   2.执行 sh i18n.sh merge
#     - 执行 goi18n merge
#     - 生成/更新 locales/translate.zh.yaml
#   3.修改translate.zh.yaml，将英文改为中文
#   4.执行 sh i18n.sh active
#     - 将 translate.zh.yaml 添加到 active.zh.yaml
#
# 前置条件：
#   1. go install -v github.com/nicksnyder/go-i18n/v2/goi18n@latest
#   2. brew install yq

set -e

CMD="$1"
LOCALES_DIR="locales"
SRC_LANG_FILE="$LOCALES_DIR/active.en.yaml"
TRANSLATE_ZH_FILE="$LOCALES_DIR/translate.zh.yaml"
ACTIVE_ZH_FILE="$LOCALES_DIR/active.zh.yaml"

case "$CMD" in
  merge)
    echo "▶ goi18n merge (yaml)..."
    if [ ! -f "$SRC_LANG_FILE" ]; then
      echo "❌ $SRC_LANG_FILE 不存在"
      exit 1
    fi
    if [ ! -f "$TRANSLATE_ZH_FILE" ]; then
      touch "$TRANSLATE_ZH_FILE"
    fi
    goi18n merge -format yaml -outdir "$LOCALES_DIR" "$SRC_LANG_FILE" "$TRANSLATE_ZH_FILE"
    echo "✅ 已生成/更新 $TRANSLATE_ZH_FILE"
    ;;
  active)
    echo "▶ 将 translate.zh.yaml 添加到 active.zh.yaml"
    if [ ! -f "$TRANSLATE_ZH_FILE" ]; then
      echo "❌ $TRANSLATE_ZH_FILE 不存在"
      exit 1
    fi
    if [ ! -f "$ACTIVE_ZH_FILE" ]; then
      cp "$TRANSLATE_ZH_FILE" "$ACTIVE_ZH_FILE"
    else
      yq eval-all --indent=4 'select(fileIndex == 0) * select(fileIndex == 1)' \
        "$ACTIVE_ZH_FILE" "$TRANSLATE_ZH_FILE" > "$ACTIVE_ZH_FILE.tmp"
      mv "$ACTIVE_ZH_FILE.tmp" "$ACTIVE_ZH_FILE"
    fi
    cp "$ACTIVE_ZH_FILE" "$TRANSLATE_ZH_FILE"
    echo "✅ 已更新 $ACTIVE_ZH_FILE"
    ;;
  *)
    echo "❌ 未知命令: $CMD"
    echo "用法: ./i18n.sh {merge|active}"
    exit 1
    ;;
esac
