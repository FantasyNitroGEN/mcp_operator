#!/bin/bash

# create-default-spec.sh - Создает базовую спецификацию для MCP сервера

set -e

SERVER_NAME="$1"
if [ -z "$SERVER_NAME" ]; then
    echo "Usage: $0 <server-name>"
    exit 1
fi

YAMLS_DIR="servers/yamls"
SOURCES_DIR="servers/sources"
SPEC_FILE="$YAMLS_DIR/$SERVER_NAME.yaml"

# Создаем директорию если не существует
mkdir -p "$YAMLS_DIR"

# Проверяем, есть ли уже спецификация
if [ -f "$SPEC_FILE" ]; then
    echo "Спецификация для $SERVER_NAME уже существует: $SPEC_FILE"
    exit 0
fi

# Определяем тип сервера на основе содержимого
SERVER_TYPE="python"
DOCKER_IMAGE="python:3.11-slim"
COMMAND='["python", "-m", "mcp_server_'$SERVER_NAME'"]'

# Проверяем наличие исходников и определяем тип
if [ -d "$SOURCES_DIR/$SERVER_NAME" ]; then
    if [ -f "$SOURCES_DIR/$SERVER_NAME/package.json" ]; then
        SERVER_TYPE="node"
        DOCKER_IMAGE="node:18-alpine"
        COMMAND='["npm", "start"]'
    elif [ -f "$SOURCES_DIR/$SERVER_NAME/go.mod" ]; then
        SERVER_TYPE="go"
        DOCKER_IMAGE="golang:1.21-alpine"
        COMMAND='["./main"]'
    elif [ -f "$SOURCES_DIR/$SERVER_NAME/Cargo.toml" ]; then
        SERVER_TYPE="rust"
        DOCKER_IMAGE="rust:1.70-slim"
        COMMAND='["./target/release/main"]'
    fi
fi

# Определяем capabilities на основе имени сервера
CAPABILITIES=""
if [[ "$SERVER_NAME" == *"filesystem"* ]] || [[ "$SERVER_NAME" == *"file"* ]]; then
    CAPABILITIES="- read_file\n  - write_file\n  - list_directory"
elif [[ "$SERVER_NAME" == *"git"* ]]; then
    CAPABILITIES="- git_operations\n  - repository_management"
elif [[ "$SERVER_NAME" == *"postgres"* ]] || [[ "$SERVER_NAME" == *"sql"* ]] || [[ "$SERVER_NAME" == *"db"* ]]; then
    CAPABILITIES="- sql_query\n  - schema_inspection"
elif [[ "$SERVER_NAME" == *"web"* ]] || [[ "$SERVER_NAME" == *"http"* ]]; then
    CAPABILITIES="- web_requests\n  - html_parsing"
else
    CAPABILITIES="- tools\n  - resources"
fi

# Создаем YAML спецификацию
cat > "$SPEC_FILE" << EOF
name: $SERVER_NAME
version: "1.0.0"
description: "MCP server for $SERVER_NAME operations"
author: "Auto-generated from Docker MCP Registry"
license: "MIT"
homepage: "https://github.com/docker/mcp-registry/tree/main/servers/$SERVER_NAME"

runtime:
  type: "$SERVER_TYPE"
  image: "$DOCKER_IMAGE"
  command: $COMMAND
  env:
    LOG_LEVEL: "info"

config:
  schema: {}
  required: []
  properties: {}

capabilities:
  $CAPABILITIES

environment:
  MCP_SERVER_NAME: "$SERVER_NAME"
  MCP_SERVER_VERSION: "1.0.0"
EOF

echo "✅ Создана спецификация: $SPEC_FILE"

# Показываем содержимое для проверки
if command -v cat >/dev/null 2>&1; then
    echo "📄 Содержимое спецификации:"
    cat "$SPEC_FILE"
fi
