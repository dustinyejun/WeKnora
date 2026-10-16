#!/bin/bash
# 本仓库定制的开发环境脚本：叠加 docker-compose.dev.custom.yml 编排容器，
# 默认本机 Go/Vite + Docker 基础服务，--docker 启用完整容器模式。
# app / frontend 命令直接交给上游 scripts/dev.sh，上游脚本保持原样以减少同步冲突。

# 设置颜色
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # 无颜色

# 获取项目根目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
COMPOSE_FILES=(-f docker-compose.dev.yml -f docker-compose.dev.custom.yml)

# 日志函数
log_info() {
    printf "%b\n" "${BLUE}[INFO]${NC} $1"
}

log_success() {
    printf "%b\n" "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    printf "%b\n" "${RED}[ERROR]${NC} $1"
}

log_warning() {
    printf "%b\n" "${YELLOW}[WARNING]${NC} $1"
}

# 选择可用的 Docker Compose 命令
DOCKER_COMPOSE_BIN=""
DOCKER_COMPOSE_SUBCMD=""

detect_compose_cmd() {
    if docker compose version &> /dev/null; then
        DOCKER_COMPOSE_BIN="docker"
        DOCKER_COMPOSE_SUBCMD="compose"
        return 0
    fi
    if command -v docker-compose &> /dev/null; then
        if docker-compose version &> /dev/null; then
            DOCKER_COMPOSE_BIN="docker-compose"
            DOCKER_COMPOSE_SUBCMD=""
            return 0
        fi
    fi
    return 1
}

# 同时加载上游 dev compose 与 custom 覆盖文件
compose() {
    "$DOCKER_COMPOSE_BIN" $DOCKER_COMPOSE_SUBCMD "${COMPOSE_FILES[@]}" "$@"
}

# 显示帮助信息
show_help() {
    printf "%b\n" "${GREEN}WeKnora 开发环境脚本（本仓库定制）${NC}"
    echo "用法: $0 [命令] [选项]"
    echo ""
    echo "命令:"
    echo "  start      启动基础服务，Go/前端使用本机源码运行"
    echo "  stop       停止所有服务"
    echo "  restart    重启所有服务"
    echo "  logs       查看服务日志"
    echo "  status     查看服务状态"
    echo "  docreader  单独重建并启动本地 Python 镜像"
    echo "  app        启动后端应用（本地运行，转交 scripts/dev.sh）"
    echo "  frontend   启动前端开发服务器（本地运行，转交 scripts/dev.sh）"
    echo "  help       显示此帮助信息"
    echo ""
    echo "start 选项:"
    echo "  --docker      从源码构建 Go/前端/Python，全部在 Docker 运行"
    echo "  --infra-only  仅启动依赖服务，Go/前端由本机运行（默认）"
    echo "  --build       重建本次启用的本地镜像（默认复用 Python 镜像）"
    echo "  --minio / --qdrant / --neo4j / --dex  启动对应可选服务"
    echo "  --langfuse    启动 Langfuse（默认已开启）"
    echo "  --no-langfuse 不启动 Langfuse"
    echo "  --odl-hybrid  启动 OpenDataLoader hybrid（Docling，镜像较大，按需启用）"
    echo "  --full        启动所有可选服务和应用容器（不含 odl-hybrid）"
    echo ""
    echo "示例："
    echo "  make dev-start DEV_ARGS=--no-langfuse    # 基础服务，本机 Go/Vite 开发"
    echo "  make dev-docker DEV_ARGS=--no-langfuse   # 从源码构建完整 Docker 开发环境"
    echo "  make dev-app / make dev-frontend         # 在另一个终端启动后端 / 前端"
}

# 加载 .env 与可选的 .env.local（后者覆盖前者），去掉行尾 \r 兼容 CRLF。
# macOS 自带 Bash 3.2 不能 source <(...)，需落到临时文件再 source。
_source_env_file() {
    local src="$1"
    local tmp
    tmp="$(mktemp)" || return 1
    sed -e 's/\r$//' "$src" > "$tmp"
    set -a
    # shellcheck source=/dev/null
    source "$tmp"
    set +a
    rm -f "$tmp"
}

load_env_files() {
    if [ -f ".env" ]; then
        _source_env_file .env || return 1
    else
        return 1
    fi

    if [ -f ".env.local" ]; then
        log_info "加载 .env.local 覆盖配置..."
        _source_env_file .env.local || return 1
    fi
    return 0
}

# 检查 Docker
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "未安装Docker，请先安装Docker"
        return 1
    fi

    if ! detect_compose_cmd; then
        log_error "未检测到 Docker Compose"
        return 1
    fi

    if ! docker info &> /dev/null; then
        log_error "Docker服务未运行"
        return 1
    fi

    return 0
}

# 检查 .env 是否启用了 hybrid 模式（用于 --odl-hybrid 启动后重建 docreader）
_should_enable_odl_hybrid_from_env() {
    local hybrid="${DOCREADER_ODL_HYBRID:-off}"
    hybrid=$(printf '%s' "$hybrid" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')
    case "$hybrid" in
        off|"") return 1 ;;
        *) return 0 ;;
    esac
}

# 等待 odl-hybrid HTTP 健康检查通过（compose 启动后服务可能仍在拉依赖）
_wait_odl_hybrid_ready() {
    local port="${ODL_HYBRID_PORT:-5002}"
    local max_wait="${ODL_HYBRID_STARTUP_WAIT_SEC:-180}"
    local waited=0
    local interval=5

    if ! command -v curl &> /dev/null; then
        log_warning "未安装 curl，跳过 odl-hybrid 就绪等待；请手动检查 http://localhost:${port}/health"
        return 0
    fi

    log_info "等待 odl-hybrid 就绪（最多 ${max_wait}s）..."
    while [ "$waited" -lt "$max_wait" ]; do
        if curl -sf "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
            log_success "odl-hybrid 已就绪 (http://localhost:${port}/health)"
            return 0
        fi
        sleep "$interval"
        waited=$((waited + interval))
    done
    log_warning "odl-hybrid 在 ${max_wait}s 内未就绪，请查看: docker logs WeKnora-odl-hybrid"
    return 1
}

# 启动服务
start_services() {
    log_info "启动开发环境..."

    check_docker || return 1
    cd "$PROJECT_ROOT"

    if ! load_env_files; then
        log_error ".env 文件不存在，请先创建"
        return 1
    fi

    if [ -n "${DEV_REMOTE_HOST:-}" ]; then
        log_warning "已配置 DEV_REMOTE_HOST=${DEV_REMOTE_HOST}，跳过本地 Docker 基础设施启动"
        log_info "接下来: make dev-app（本地后端）或 make dev-frontend（前端）"
        return 0
    fi

    # 解析 profile 参数
    shift  # 移除 "start" 命令本身
    # 默认启动基础服务 + langfuse，Go/Vite 在本机运行。
    PROFILES="--profile langfuse"
    ENABLED_SERVICES="langfuse"
    local app_containers=false
    local rebuild=false
    while [ $# -gt 0 ]; do
        case "$1" in
            --docker)
                PROFILES="$PROFILES --profile app"
                app_containers=true
                ;;
            --infra-only)
                PROFILES="${PROFILES//--profile app/}"
                app_containers=false
                ;;
            --build)
                rebuild=true
                ;;
            --minio|--qdrant|--neo4j|--dex|--langfuse)
                PROFILES="$PROFILES --profile ${1#--}"
                ENABLED_SERVICES="$ENABLED_SERVICES ${1#--}"
                ;;
            --no-langfuse)
                PROFILES="${PROFILES//--profile langfuse/}"
                ENABLED_SERVICES="${ENABLED_SERVICES//langfuse/}"
                ;;
            --odl-hybrid)
                if [[ "$ENABLED_SERVICES" != *"odl-hybrid"* ]]; then
                    PROFILES="$PROFILES --profile odl-hybrid"
                    ENABLED_SERVICES="$ENABLED_SERVICES odl-hybrid"
                fi
                ;;
            --full)
                PROFILES="--profile full"
                ENABLED_SERVICES="minio qdrant neo4j dex"
                app_containers=true
                break
                ;;
            *)
                log_warning "未知参数: $1"
                ;;
        esac
        shift
    done

    # 本机模式停止占用 8080/5173 的应用容器，保留基础服务和数据卷。
    # Python 仅在镜像不存在或显式 --build 时构建，避免启动基础服务也重编译。
    if [ "$app_containers" = false ]; then
        log_info "本机开发模式：释放 Go/Vite 端口，复用基础服务..."
        compose --profile app stop frontend app || return 1
        if [ "$rebuild" = false ] && ! docker image inspect weknora-docreader:dev-local >/dev/null 2>&1; then
            log_info "首次构建本地 docreader 镜像..."
            compose build docreader || return 1
        fi
    fi

    # 独立本地标签保证使用本地源码镜像；完整 Docker 模式按当前源码重建。
    export VERSION_ARG="$(tr -d '\r\n' < VERSION)"
    export COMMIT_ID_ARG="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
    export VITE_FRONTEND_COMMIT="${VITE_FRONTEND_COMMIT:-$COMMIT_ID_ARG}"
    local build_option=--no-build
    if [ "$app_containers" = true ] || [ "$rebuild" = true ]; then
        build_option=--build
    fi
    compose $PROFILES up -d "$build_option" --wait --wait-timeout 300
    local compose_rc=$?
    if [ "$compose_rc" -eq 0 ] && [[ "$ENABLED_SERVICES" == *"odl-hybrid"* ]]; then
        log_info "构建/更新 odl-hybrid 镜像..."
        compose $PROFILES up -d --build odl-hybrid
        _wait_odl_hybrid_ready || true
        # docreader 需读取 DOCREADER_ODL_HYBRID；若刚改 .env，强制重建以注入环境变量
        if _should_enable_odl_hybrid_from_env; then
            log_info "重建 docreader 以应用 DOCREADER_ODL_HYBRID=${DOCREADER_ODL_HYBRID} ..."
            compose up -d --force-recreate docreader
        fi
    fi

    if [ "$compose_rc" -ne 0 ]; then
        log_error "服务启动失败"
        return 1
    fi

    log_success "开发环境已启动"
    echo ""
    log_info "服务访问地址:"
    if [ "$app_containers" = true ]; then
        echo "  - Web UI:       http://localhost:${DEV_FRONTEND_PORT:-5173}"
        echo "  - Go API:       http://localhost:${DEV_APP_PORT:-8080}"
    fi
    echo "  - PostgreSQL:    localhost:5432"
    echo "  - Redis:         localhost:6379"
    echo "  - DocReader:     localhost:50051"
    [[ "$ENABLED_SERVICES" == *"minio"* ]] && echo "  - MinIO:         localhost:9000 (Console: localhost:9001)"
    [[ "$ENABLED_SERVICES" == *"qdrant"* ]] && echo "  - Qdrant:        localhost:6333 (gRPC: localhost:6334)"
    [[ "$ENABLED_SERVICES" == *"neo4j"* ]] && echo "  - Neo4j:         localhost:7474 (Bolt: localhost:7687)"
    [[ "$ENABLED_SERVICES" == *"dex"* ]] && echo "  - Dex:           localhost:5556"
    [[ "$ENABLED_SERVICES" == *"langfuse"* ]] && echo "  - Langfuse:      http://localhost:${LANGFUSE_WEB_PORT:-3000}"
    if [[ "$ENABLED_SERVICES" == *"odl-hybrid"* ]]; then
        echo "  - ODL Hybrid:    http://localhost:${ODL_HYBRID_PORT:-5002} (health: /health)"
        echo "                   docreader 需 DOCREADER_ODL_HYBRID=docling-fast"
    fi

    echo ""
    log_info "接下来的步骤:"
    if [ "$app_containers" = true ]; then
        printf "%b\n" "${YELLOW}打开网页:${NC} http://localhost:${DEV_FRONTEND_PORT:-5173}"
        log_info "修改源码后再次运行 make dev-docker，按构建缓存更新镜像和容器。"
    else
        printf "%b\n" "${YELLOW}1. 在新终端运行后端:${NC} make dev-app"
        printf "%b\n" "${YELLOW}2. 在新终端运行前端:${NC} make dev-frontend"
    fi
    return 0
}

# Python 修改后只更新解析服务，不构建 Go/前端，也不切换应用运行模式。
rebuild_docreader() {
    check_docker || return 1
    cd "$PROJECT_ROOT"
    load_env_files || return 1
    compose up -d --no-deps --build --wait --wait-timeout 300 docreader
}

# 停止服务（包含所有 profile 下的容器）
stop_services() {
    log_info "停止开发环境服务..."
    check_docker || return 1
    cd "$PROJECT_ROOT"
    if compose --profile '*' down; then
        log_success "所有服务已停止"
        return 0
    fi
    log_error "服务停止失败"
    return 1
}

# 重启服务，start 选项原样传递
restart_services() {
    shift
    stop_services || return 1
    sleep 2
    start_services start "$@"
}

show_logs() {
    check_docker || return 1
    cd "$PROJECT_ROOT"
    compose --profile '*' logs -f
}

show_status() {
    check_docker || return 1
    cd "$PROJECT_ROOT"
    compose --profile '*' ps
}

# 解析命令
CMD="${1:-help}"
case "$CMD" in
    start)
        start_services "$@"
        ;;
    stop)
        stop_services
        ;;
    restart)
        restart_services "$@"
        ;;
    logs)
        show_logs
        ;;
    status)
        show_status
        ;;
    docreader)
        rebuild_docreader
        ;;
    app|frontend)
        exec "$SCRIPT_DIR/dev.sh" "$@"
        ;;
    help|--help|-h)
        show_help
        ;;
    *)
        log_error "未知命令: $CMD"
        show_help
        exit 1
        ;;
esac
