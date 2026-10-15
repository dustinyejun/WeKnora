# AGENTS.md

本文件是面向 AI 编码代理（Claude Code、Codex、Gemini CLI 等）的仓库说明，统一在这里维护。Claude Code 通过根目录 `CLAUDE.md` 里的 `@AGENTS.md` 引用本文件，不要在 `CLAUDE.md` 中重复内容。`cli/` 子模块另有自己的 `cli/AGENTS.md`。

## 仓库概览

WeKnora 是腾讯开源的 LLM 知识库框架（RAG + Agent + Wiki）。仓库由几个可独立开发的部分组成：

| 部分 | 目录 | 技术 |
| --- | --- | --- |
| 主后端 `app` | `cmd/server` + `internal/` | Go 1.26，Gin + GORM + uber/dig，需要 CGO（DuckDB、sqlite-vec） |
| 文档解析服务 `docreader` | `docreader/` | Python ≥ 3.10，gRPC，依赖用 uv 管理（`uv.lock`） |
| 前端 | `frontend/` | Vue 3 + Vite 7 + TypeScript + Pinia + TDesign |
| CLI `weknora` | `cli/`（独立 Go module） | 有自己的 `cli/AGENTS.md`，构建、测试、代码风格约定都在那里 |
| Go SDK | `client/`（独立 Go module） | CLI 和 mcp-server 的底层客户端 |
| 桌面版 | `cmd/desktop` | Wails，build tag `desktop` |
| 文档站 | `website-docs/` | 产品、部署、API、开发文档的唯一维护位置；`docs/` 只剩 swagger 生成物 |

## 常用命令

### Go 后端（仓库根目录）

```bash
make build                  # go build -o WeKnora ./cmd/server
make fmt                    # go fmt ./...；CI 用 gofmt -l 只检查改动过的文件
make lint                   # golangci-lint run（全仓库有大量历史告警）
golangci-lint run --new-from-merge-base=origin/main   # 只看自己引入的问题，与 PR CI 一致
make docs                   # swag 重新生成 docs/{docs.go,swagger.json,swagger.yaml}；改了 handler 注解后必须执行
make deps                   # go mod download

go test ./internal/application/service/                   # 单个包
go test -run 'TestName' ./internal/application/service/   # 单个测试
go vet $(go list ./... | grep -v /docreader/)             # 与 CI 相同
go test $(go list ./... | grep -v /docreader/)            # 与 CI 相同的全量测试
go test -tags desktop ./internal/container                # CI 额外跑的桌面版装配测试
```

- `make test`（`go test -v ./...`）会连带 `./docreader/client`，那组测试要求本机 50051 端口上有运行中的 docreader gRPC 服务，否则失败；CI 用 `grep -v /docreader/` 排除。
- 单元测试默认用 SQLite 内存库，不需要 Docker。设置 `WEKNORA_REPOSITORY_TEST_POSTGRES_DSN` 才会跑 Postgres 专用用例。带 `integration`、`docker_integration`、`e2b_integration`、`feishu_integration` build tag 的测试需要外部服务，日常不跑。
- CI 设置 `LOG_FORMAT=""`，避免本机日志模板影响测试。
- 第一次编译要构建 DuckDB 等 CGO 依赖，很慢，属正常现象。

### 数据库迁移

```bash
make migrate-create name=xxx    # 在 migrations/versioned 下生成 .up/.down.sql（需要 golang-migrate CLI）
make migrate-up                 # 也有 migrate-down / migrate-version / migrate-goto version=N
make migrate-force version=N    # 修复 dirty 状态
```

- Postgres 用 `migrations/versioned/`，SQLite（Lite 与桌面版）用 `migrations/sqlite/`，两套目录**编号独立**；改表结构时两边都要加。
- 应用启动时自动执行迁移（`AUTO_MIGRATE=false` 关闭，`AUTO_RECOVER_DIRTY` 控制 dirty 自动恢复），逻辑在 `internal/database/migration.go`。
- `internal/database/migration_versions_test.go` 会加载两个目录，防止两个分支抢同一编号。

### 前端（在 `frontend/` 下）

```bash
npm ci
npm run dev             # Vite，5173 端口；/api、/files、/mcp/ 代理到 VITE_DEV_PROXY_TARGET（默认 http://localhost:8080）
npm test                # tsx --test，跑所有 *.test.ts（含 i18n 键一致性审计）
npx tsx --test src/stores/uploadQueue.test.ts   # 单个测试文件
npm run type-check      # vue-tsc --build
npm run build
npm run check-i18n      # 只跑六种语言的 locale 键审计
```

### docreader（在仓库根目录运行）

```bash
uv sync --project docreader
uv run --project docreader python -m docreader.main      # 启动 gRPC 服务，默认 50051
uv run --project docreader python -m unittest discover -s docreader/tests -p "test_*.py" -v
uv run --project docreader python -m unittest discover -s docreader/tests -p "test_config.py"   # 单个文件
```

测试里 `from docreader import ...` 依赖仓库根目录在 `sys.path` 上，所以要在根目录运行。改了 `docreader/proto/docreader.proto` 后，在 `docreader/` 下执行 `make proto` 同时重新生成 Python 和 Go 代码。

### 本地开发模式

```bash
cp .env.example .env                       # 首次；.env.local 可覆盖
make dev-start DEV_ARGS=--no-langfuse      # Docker 启动 postgres/redis/docreader（默认还带 Langfuse）
make dev-app                               # 新终端：本地跑 Go 后端（装了 air 就热重载；检测到 anydoc 静态库会自动加 -tags anydoc）
make dev-frontend                          # 新终端：本地跑 Vite
make dev-docker DEV_ARGS=--no-langfuse     # 本 fork 专有：用当前源码构建 app/ui 镜像，和基础服务一起跑
make dev-status / dev-logs / dev-stop
```

`make dev-app` 会把 `DB_HOST`、`REDIS_ADDR`、`DOCREADER_ADDR` 等覆盖为 127.0.0.1，上传文件落在 `.local-data/files`。两种本地运行方式的切换、Neo4j 注意事项见 `CUSTOM_LOCAL_RUN.md`。

### 其他构建

```bash
make build-lite       # 单二进制 Lite 版：SQLite + 内存队列，tags sqlite_fts5，先把前端构建到 web/（SKIP_FRONTEND=1 跳过）
make run-lite         # 需要 .env.lite（模板 .env.lite.example）
make anydoc-lib       # Rust 构建 anydoc 静态库；之后 go build -tags anydoc 链接进程内 Office 解析引擎
make model-catalog-generate   # 重新生成模型目录；make model-catalog-check 与 CI 一致
./scripts/install-git-hooks.sh   # pre-commit: gofmt + lint；pre-push: vet/test/build。SKIP_HOOKS=1 跳过，HOOK_SKIP_TEST=1 跳过测试
```

## 后端架构

### 启动与依赖注入

`cmd/server/main.go` → `container.BuildContainer()`（`internal/container/container.go`，dig，两百多个 Provide）→ 自动迁移 → `runStartupBootstrap`（best-effort 的一次性钩子）→ `router.NewRouter`。新增 service、repository、handler 都要在 `container.go` 注册，handler 还要加进 `router.RouterParams`（`dig.In`）。检索引擎按 VectorStore 配置动态构造的逻辑在 `internal/container/engine_factory.go`。

### 分层

- `internal/handler/`：Gin HTTP 层。错误用 `c.Error(apperrors.NewXxxError(...))` 抛出，`middleware.ErrorHandler` 统一转成 `{success:false, error:{code,message,details}}`；错误码定义在 `internal/errors/`。
- `internal/application/service/`：业务层。`internal/application/repository/`：GORM 数据访问，`repository/retriever/<engine>/` 是各检索后端。`internal/application/access/`：与 Gin 无关的共享权限判定，README 列了每条规则的边界。
- 所有 repository 和 service 接口集中在 `internal/types/interfaces/`，领域模型在 `internal/types/`。依赖一律通过接口注入。
- 请求上下文：认证中间件把身份同时写进 gin keys 和 `c.Request.Context()`（`middleware/auth_context.go`），读取用 `types.TenantIDFromContext`、`UserIDFromContext` 等 helper（`types/context_helpers.go`）。租户隔离依赖 context 里的 TenantID，repository 层不做隐式过滤时必须由调用方传入租户。

### 路由与 RBAC

路由按领域拆在 `internal/router/routes_*.go`，每条路由显式串接 guard：角色 guard `g.Viewer()/Contributor()/Admin()/Owner()`，所有权 guard `g.OwnedKBOrAdmin()` 等（创建者或 Admin+），KB 共享范围 `g.KBAccessRead/WriteFrom...`，API key 能力 `apiKeyGroup/apiKeyRoute`。`internal/router/rbac.go` 开头的注释解释了怎样选 guard，这是本仓库 RBAC bug 的主要来源，加新路由前先读。

### 对话管道（RAG）

`internal/application/service/chat_pipeline/`：`EventManager` 按 `types.EventType`（定义在 `types/chat_manage.go`，如 `load_history`、`query_understand`、`chunk_search`、`chunk_rerank`、`chunk_merge`、`chat_completion_stream`）触发插件链。每个插件实现 `Plugin.OnEvent(ctx, event, chatManage, next)` 并声明 `ActivationEvents()`，在构造函数里 `eventManager.Register(plugin)`。阶段间共享状态都挂在 `types.ChatManage` 上。

### Agent

`internal/agent/`：无状态 ReAct 引擎（`think.go`、`act.go`、`observe.go`、`steer.go`、`finalize.go`），对话历史每轮由 service 从 DB 重建后传入。工具在 `internal/agent/tools/` 通过 `ToolRegistry` 注册；`skills/` 负责技能加载，`compaction/` 上下文压缩，`approval/` MCP 工具审批。沙箱后端（Docker、E2B、Cube、桌面、本地 seatbelt）在 `internal/sandbox/` 和 `internal/localsandbox/`。

### 检索引擎

`service/retriever/registry.go` 的 `RetrieveEngineRegistry` 同时维护两套注册：按引擎类型（环境变量 `RETRIEVE_DRIVER`）和按 VectorStore 实例（DB 表）。实例缺失时用 `EngineFactory` 懒构建，带 singleflight 和失败冷却。支持 postgres（ParadeDB）、sqlite、elasticsearch v7/v8、opensearch、qdrant、milvus、weaviate、doris、tencentvectordb；图谱走 neo4j。

### 异步任务

基于 asynq（Redis）。队列拓扑的唯一来源是 `internal/types/task.go` 的 `QueueDefinitions`（队列 → worker pool → task types）；新任务类型必须在这里声明队列，否则 `task_queue_test.go` 失败。处理器在 `internal/router/task.go` 注册到各 pool 的 asynq server。Lite 模式没有 Redis，用 `router/sync_task.go` 的 `SyncTaskExecutor` 在 goroutine 内执行同一组处理器。文档解析、向量化、图谱抽取、数据源同步、Wiki 都走这条路。

### 文档解析

分两层。Go 侧 `internal/infrastructure/docparser/engines.go` 注册解析引擎：`builtin`（docreader gRPC）、`simple`（Go 原生文本和图片）、`anydoc`（进程内 Rust，需 build tag）以及 MinerU、PaddleOCR-VL、WeKnora Cloud 等远程引擎。Python 侧 `docreader/parser/registry.py` 按文件扩展名把 builtin 引擎映射到各 parser 类。分块策略在 `internal/infrastructure/chunker/`。

### 模型目录

- 厂商定义是 Go 代码：`internal/models/providers/<vendor>.go`，在 `builtin.go` 的 `Builtins()` 列表登记。
- 模型元数据是数据：`internal/models/catalog/data/seed.json`（固定来源）+ `overrides.json`（协议覆盖）→ `make model-catalog-generate` 生成 `models.generated.json`。**不要手改 generated 文件**，CI 用 `scripts/model-catalog/generate.py --check` 校验。
- 部署层覆盖：`config/models.json`（见 `.example`），由 `internal/models/runtime` 叠加到内置目录之上。
- 各模型类型在 `internal/models/{chat,embedding,rerank,vlm,asr}/`，统一包了 langfuse 追踪与并发限流 wrapper；协议适配在 `internal/models/api/`。

### 可插拔扩展

- 数据源连接器：`internal/datasource/connector/<type>/`，实现 `Connector` 接口并在 `container.go` 注册，步骤见 `internal/datasource/CONNECTOR_IMPLEMENTATION_GUIDE.md`。
- IM 渠道 `internal/im/<platform>/`，联网搜索 `internal/infrastructure/web_search/`，MCP 客户端 `internal/mcp/`，内置 MCP Server `internal/mcpserver/`。
- 完整扩展点清单（解析器、分块、检索引擎、模型厂商、搜索引擎、连接器）见 `website-docs/06-development/03-extension-points.md`。

### 配置

`config/config.yaml` 由 viper 加载（当前目录或 `./config`），解析前先把 `${ENV_VAR}` 替换成环境变量；大部分运行参数来自 `.env`（`.env.example` 按分区有详细注释）。提示词在 `config/prompt_templates/*.yaml`，`config.yaml` 用 `*_prompt_id` 引用。版本、Edition、CommitID 通过 ldflags 注入 `internal/handler` 的包变量（`scripts/get_version.sh ldflags`）。

## 前端结构

`src/api/`（按领域的 axios 封装，基础实例在 `utils/request.ts`）、`src/stores/`（Pinia）、`src/views/`（页面）、`src/components/`、`src/composables/`。入口 `main.ts`；`embed-main.ts` 是嵌入外站 widget 的独立入口。i18n 六种语言在 `src/i18n/locales/`，新增文案必须补齐所有语言，`localeKeyAudit.test.ts` 会在 `npm test` 中检查键的一致性。

## Build tags

`anydoc`（进程内 Office 解析，需先 `make anydoc-lib`）、`desktop`（Wails 桌面版）、`sqlite_fts5`（Lite 全文检索）、`integration` 系列（需外部服务的测试）。`internal/localsandbox` 只被 `desktop` 构建引用，所以 `make docs` 把它排除在 swag 扫描之外。

## 代码规范

- golangci-lint v2（`.golangci.yml`）：`lll` 行宽 120、`revive`、`govet`；格式化 gofmt + gofumpt。PR CI 用 `--new-from-merge-base` 只报新增问题。
- 新的产品或开发说明放 `website-docs/`，不要再往 `docs/` 放。
- `cli/` 有独立的风格约定（Options + 窄接口 Service 模式、testify、不在 Go 源码里用 em-dash），改 CLI 前读 `cli/AGENTS.md`。

## 分支与定制

本仓库是 Tencent/WeKnora 的 fork。`main` 只做上游镜像，所有自己的改动都提交到当前所在的定制分支。

- 同步上游：在定制分支上运行 `git sync-upstream`（仓库级 alias，`gh repo sync` 之后把 `origin/main` merge 进当前分支）。只 merge，不 rebase，不 force push，不往 `main` 提交。
- 定制内容只放在上游不会碰的独立文件：`GNUmakefile`、`scripts/dev-custom.sh`、`docker-compose.dev.custom.yml`、`docker-compose.override.yml`、`CUSTOM_DEV.md`、`CUSTOM_LOCAL_RUN.md`。除 `.dockerignore` 和 `docker/Dockerfile.app` 的小改动外不修改官方文件，以减少合并冲突。
- `AGENTS.md`（本文件）也是定制文件，随定制分支提交。`CLAUDE.md` 只是引用入口，被上游 `.gitignore` 的 `Claude.md` 规则忽略（macOS 文件系统不区分大小写），只存在于本机，不入库。
- `GNUmakefile` 会拦截 `dev-start`、`dev-local`、`dev-docker`、`dev-docreader`、`dev-stop`、`dev-restart`、`dev-logs`、`dev-status` 转给 `scripts/dev-custom.sh`，其余目标（含 `dev-app`、`dev-frontend`）仍走上游 `Makefile`。一条 make 命令里不要混用两类目标。
- 开发环境与正式环境（`docker compose up`）共用 compose 项目名和 postgres 数据卷，不能同时运行。Neo4j 用正式环境的 `WeKnora-neo4j` 容器，开发命令不要加 `--neo4j`。详见 `CUSTOM_DEV.md`。
