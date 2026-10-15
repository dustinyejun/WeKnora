# 本仓库定制说明

本仓库的定制分支在官方代码之上增加了本地开发和部署方式。为减少同步上游时的冲突，定制内容都放在上游不会修改的独立文件里，官方文件尽量保持原样。

## 文件

| 文件 | 作用 |
| --- | --- |
| `GNUmakefile` | GNU make 优先读取它。`dev-start` 等开发命令改走 `scripts/dev-custom.sh`，其余目标原样交给上游 `Makefile` |
| `scripts/dev-custom.sh` | 叠加 `docker-compose.dev.custom.yml` 编排容器；`app`、`frontend` 直接交给上游 `scripts/dev.sh` |
| `docker-compose.dev.custom.yml` | 开发环境覆盖配置：Go/前端容器、本地构建 docreader、共用正式环境 postgres 数据卷、Neo4j 内存限制 |
| `docker-compose.override.yml` | 不带 `-f` 运行 `docker compose` 时自动合并，给正式环境的 Neo4j 加内存限制 |
| `CUSTOM_LOCAL_RUN.md` | 两种本地运行方式的说明 |

只有两处直接改了官方文件，都是很少变动的位置：

- `.dockerignore`：排除 `.local-data/`。上游 `dev.sh` 把本机运行时上传的文件放在这里，而 `Dockerfile.app` 会 `COPY . .`。
- `docker/Dockerfile.app`：Go 构建步骤加 go-build 缓存和 `GO_BUILD_PARALLELISM`，Docker 虚拟机内存较小时不容易编译失败。

## 本地运行

两种本地运行方式（app 和前端用源码直接运行、打包成镜像运行）的完整步骤见 [CUSTOM_LOCAL_RUN.md](CUSTOM_LOCAL_RUN.md)。

## 可选环境变量（写在 `.env`）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DEV_APP_PORT` / `DEV_FRONTEND_PORT` | `8080` / `5173` | 完整 Docker 模式下的宿主机端口；容器之间始终通过 `app:8080`、`docreader:50051` 访问 |
| `GO_BUILD_PARALLELISM` | `2` | 构建 app 镜像时同时编译的 Go 包数量，Docker 虚拟机内存小时调低 |
| `DEV_NEO4J_URI` | `bolt://host.docker.internal:7687` | app 容器访问的 Neo4j，默认用宿主机上已发布端口的 Neo4j |
| `NEO4J_HEAP_SIZE` | `512m` | Neo4j JVM 堆的初始值和最大值 |
| `NEO4J_PAGECACHE_SIZE` | `256m` | Neo4j 页缓存 |
| `NEO4J_MEMORY_LIMIT` | `1536m` | Neo4j 容器内存上限，要比堆加页缓存大，给堆外内存、线程、索引留空间 |

## 部署（正式环境）

本分支已经改了应用代码（会话分享、脑图、品牌文案等），部署必须用当前源码构建镜像，直接拉官方镜像会丢掉这些功能。下面的命令在仓库根目录运行，会自动叠加 `docker-compose.override.yml`：

```bash
make dev-stop                                  # 先停开发环境
docker compose --profile neo4j up -d --build   # 用当前源码构建带 build 配置的服务（app、frontend、docreader、cpolar 等）并启动
docker compose --profile neo4j ps              # 等服务变成 healthy/running
# 页面 http://localhost（FRONTEND_PORT），API http://localhost:8080（APP_PORT）
```

- **升级**：先 `git sync-upstream` 合并上游，再执行上面的 `up -d --build`。`.env` 里的 `WEKNORA_VERSION` 只影响 `pull` 官方镜像，本分支用不到。
- **停止**：`docker compose --profile neo4j down`。不要加 `-v`，否则会删除数据卷。
- **不要再执行 `docker compose pull`**：本地构建出的镜像占用官方标签 `wechatopenai/weknora-*:latest`，`pull` 会用官方镜像覆盖掉本分支的改动。

## 历史数据库：旧的分享迁移编号

会话分享表现在由应用启动时幂等建表（`internal/database/conversation_share_schema.go`），不占用迁移编号，`AUTO_MIGRATE=false` 时也会执行。但在 2026-10-13 到 2026-10-15 之间，本分支曾把分享表做成编号迁移 versioned `000119` / sqlite `000037`，与随后合入的上游 `000119_agent_prompts` / `000037_agent_prompts` 同号。只有在这个窗口内用本分支启动过应用的数据库会受影响：`schema_migrations` 停在 119（SQLite 为 37），却没有 `agent_prompts` 表和 `messages.run_state` 列，新消息写入会失败。本机 `weknora_postgres-data` 卷已核实停在版本 114，从未执行过任何分享迁移，不受影响；把本分支部署到其他已有数据库时，启动后按下面检查一次。

```bash
# 正式环境容器名是 WeKnora-postgres，开发环境是 WeKnora-postgres-dev
docker exec WeKnora-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "select version, dirty from schema_migrations" -c "\dt agent_prompts"'
```

若版本为 119 且 `agent_prompts` 不存在，把版本退回 118 再重启 app，自动迁移会补跑上游的 119；分享表由幂等建表保留，不会丢数据：

```bash
docker exec WeKnora-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -c "update schema_migrations set version = 118, dirty = false"'
docker compose --profile neo4j restart app
```

SQLite（Lite 版、桌面版）对应的是把 37 退回 36：`sqlite3 <db 文件> "update schema_migrations set version = 36, dirty = 0"`。另外，幂等建表的第一版把 `created_at` 声明成了 `TIMESTAMP WITH TIME ZONE`，go-sqlite3 读不回这种声明；现在 `EnsureConversationShareSchema` 启动时会检查该列的声明类型，不是 `DATETIME`、`TIMESTAMP`、`DATE` 之一的表会分别在各自的事务内重建，保留全部行和附件内容并恢复唯一索引，某张表重建失败只回滚这张表，不需要手工删表。

## 关闭自动迁移时的分享表

分享表不在编号迁移里，`make migrate-up` 不会创建它们。应用账户有建表权限时，启动时的幂等建表会自动完成，`AUTO_MIGRATE=false` 也一样。只有应用账户没有 DDL 权限、迁移由独立账户执行的部署，才需要用迁移账户手工执行 `migrations/manual/` 下对应方言的 SQL。语句全部幂等，可以重复执行：

```bash
docker exec -i WeKnora-postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < migrations/manual/conversation_shares_postgres.sql
# SQLite（Lite 版、桌面版）：sqlite3 <db 文件> < migrations/manual/conversation_shares_sqlite.sql
```

这两个文件由 Go 里的表定义生成，`TestConversationShareManualSQLInSync` 保证两者一致。改了 `internal/database/conversation_share_schema.go` 的表定义后，运行 `UPDATE_SHARE_SQL=1 go test -run TestConversationShareManualSQLInSync ./internal/database/` 重新生成。

## 注意

- **开发环境和正式环境不要同时运行**：两个 compose 文件都没有设置项目名，都属于同一个 compose 项目 `weknora`。同名服务（postgres、redis、app 等）会互相替换容器，`make dev-stop` 也会把正式环境的 Neo4j 容器一起删掉（数据卷保留）。切换前先停掉当前这一套。
- **两套环境共用 postgres 数据**：开发环境挂载的是正式环境的 `weknora_postgres-data` 卷，部署后能看到开发时的数据，反之亦然。
- **不要再改官方文件**：新的定制内容放进上面这些独立文件。同步上游的方法：在定制分支上运行 `git sync-upstream`，再 `git push`。
