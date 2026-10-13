# custom_dev 本地开发定制

`custom_dev` 分支在官方代码之上增加了本地开发方式。为减少同步上游时的冲突，定制内容都放在上游不会修改的独立文件里，官方文件尽量保持原样。

## 文件

| 文件 | 作用 |
| --- | --- |
| `GNUmakefile` | GNU make 优先读取它。`dev-start` 等开发命令改走 `scripts/dev-custom.sh`，其余目标原样交给上游 `Makefile` |
| `scripts/dev-custom.sh` | 叠加 `docker-compose.dev.custom.yml` 编排容器；`app`、`frontend` 直接交给上游 `scripts/dev.sh` |
| `docker-compose.dev.custom.yml` | 开发环境覆盖配置：Go/前端容器、本地构建 docreader、共用正式环境 postgres 数据卷、Neo4j 内存限制 |
| `docker-compose.override.yml` | 不带 `-f` 运行 `docker compose` 时自动合并，给正式环境的 Neo4j 加内存限制 |

只有两处直接改了官方文件，都是很少变动的位置：

- `.dockerignore`：排除 `.local-data/`。上游 `dev.sh` 把本机运行时上传的文件放在这里，而 `Dockerfile.app` 会 `COPY . .`。
- `docker/Dockerfile.app`：Go 构建步骤加 go-build 缓存和 `GO_BUILD_PARALLELISM`，Docker 虚拟机内存较小时不容易编译失败。

## 两种开发模式

**本机模式（默认）**：Go 和 Vite 在本机运行，docreader 与基础设施在 Docker 中运行。修改 Go/Vue 代码不需要构建镜像。

```bash
make dev-start DEV_ARGS=--no-langfuse   # 启动基础服务，并停掉占用 8080/5173 的应用容器
make dev-app                            # 终端 1：Go（装了 air 会自动重载）
make dev-frontend                       # 终端 2：Vite
# 页面 http://localhost:5173，后端 API http://localhost:8080

make dev-docreader                      # 修改了 Python 代码后，只重建解析服务
```

`make dev-local` 和 `make dev-start` 相同。已有本地 docreader 镜像时直接复用，第一次缺少镜像才从源码构建。

**完整 Docker 模式**：Go、前端、docreader 都从当前源码构建，全部在容器里运行。先在本机的 Go/Vite 终端按 Ctrl+C 释放端口，再执行：

```bash
make dev-docker DEV_ARGS=--no-langfuse  # 修改源码后再次执行，按构建缓存更新镜像
```

三个应用使用本地标签 `weknora-app:dev-local`、`weknora-ui:dev-local`、`weknora-docreader:dev-local`，不使用官方镜像。前端容器用 NGINX 提供页面，并把 API 转发到 `app:8080`。

两种模式共用 `.local-data/files` 和数据库、Redis 数据卷，切换模式不会删除数据。切回本机模式只需 `make dev-local`，基础服务不用停。

其他命令：`make dev-stop`、`make dev-restart DEV_ARGS=...`、`make dev-logs`、`make dev-status`。这些命令覆盖所有 profile 下的容器。`start` 的全部选项见 `./scripts/dev-custom.sh help`。

注意：一条 make 命令里的目标都属于上面这些开发命令时，才会走定制版。像 `make dev-stop build` 这样混着写，会整体交给上游 `Makefile`，`dev-stop` 也会变成官方版本。

## 可选环境变量（写在 `.env`）

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DEV_APP_PORT` / `DEV_FRONTEND_PORT` | `8080` / `5173` | 完整 Docker 模式下的宿主机端口；容器之间始终通过 `app:8080`、`docreader:50051` 访问 |
| `GO_BUILD_PARALLELISM` | `2` | 构建 app 镜像时同时编译的 Go 包数量，Docker 虚拟机内存小时调低 |
| `DEV_NEO4J_URI` | `bolt://host.docker.internal:7687` | app 容器访问的 Neo4j，默认用宿主机上已发布端口的 Neo4j |
| `NEO4J_HEAP_SIZE` | `512m` | Neo4j JVM 堆的初始值和最大值 |
| `NEO4J_PAGECACHE_SIZE` | `256m` | Neo4j 页缓存 |
| `NEO4J_MEMORY_LIMIT` | `1536m` | Neo4j 容器内存上限，要比堆加页缓存大，给堆外内存、线程、索引留空间 |

## 注意

- **两套 postgres 不要同时启动**：开发环境的 postgres 挂载的是正式环境的 `weknora_postgres-data` 卷。先跑了 `make dev-start`，又跑 `docker compose up -d`，两个 postgres 会同时写同一份数据，可能损坏数据库。
- **不要再改官方文件**：新的定制内容放进上面这些独立文件。同步上游的方法：在 `custom_dev` 上运行 `git sync-upstream`，再 `git push`。
