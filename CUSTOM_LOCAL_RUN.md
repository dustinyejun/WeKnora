# 本地运行：源码方式与镜像方式

在本机运行自己的代码有两种方式。两者的区别只在于 Go 后端（app）和前端（frontend）在哪里运行：

| | 方式一：源码直接运行 | 方式二：打包成镜像运行 |
| --- | --- | --- |
| Go 后端、前端 | 本机进程（Go/air、Vite） | Docker 容器，用当前源码构建 |
| 基础服务 | Docker | Docker（与方式一是同一套容器） |
| 改代码后 | 后端自动重新编译，前端热更新 | 重新运行 `make dev-docker` |
| 适用场景 | 日常开发、调试 | 验证打包后的镜像，更接近部署环境 |

两种方式的页面地址都是 http://localhost:5173 ，后端 API 都在 8080 端口。

## 准备

- 仓库根目录要有 `.env`。没有的话先运行 `cp .env.example .env`，再填写必填项。
- Docker 已经启动。
- 方式一还需要本机装好 Go 1.26+ 和 Node.js 22+（含 npm）。如果装了 air（`go install github.com/air-verse/air@latest`），改 Go 代码后会自动重启。
- Neo4j 已经在运行，见下文 [Neo4j](#neo4j)。

## 基础服务（两种方式共用）

`make dev-start` 和 `make dev-docker` 都会先启动下面这些容器。两种方式用的是同一套容器和数据：

| 服务 | 端口 | 说明 |
| --- | --- | --- |
| postgres | 5432 | 挂载正式环境的 `weknora_postgres-data` 数据卷 |
| redis | 6379 | |
| docreader | 50051 | Python 解析服务，用本地源码构建成 `weknora-docreader:dev-local` |
| Langfuse | 3000 | 链路追踪，默认启动。加 `DEV_ARGS=--no-langfuse` 就不启动，能省不少内存 |

上传的文件在两种方式下都存放在仓库内的 `.local-data/files`。

修改了 Python 解析代码时，运行 `make dev-docreader`，只重建解析服务。两种方式都适用。

### Neo4j

`.env` 里设置了 `NEO4J_ENABLE=true`。开发命令不会启动 Neo4j，两种方式都连接宿主机 7687 端口上的 `WeKnora-neo4j`，也就是正式环境的 Neo4j 容器。

```bash
docker ps --filter name=WeKnora-neo4j        # 查看是否在运行
docker compose --profile neo4j up -d neo4j   # 没在运行时启动它
```

不要给开发命令加 `--neo4j`。加了会启动开发版 Neo4j 并替换掉 `WeKnora-neo4j`。开发版使用另一个没有数据的数据卷，图谱数据看起来就像丢了。

## 方式一：源码直接运行

```bash
make dev-start DEV_ARGS=--no-langfuse   # 启动基础服务，完成后命令结束
make dev-app                            # 新开终端 A：Go 后端，运行期间一直占用该终端
make dev-frontend                       # 新开终端 B：Vite 前端，运行期间一直占用该终端
```

- `make dev-start` 会顺便停掉方式二留下的 app 和前端容器，空出 8080 和 5173 端口。如果还没有本地 docreader 镜像，第一次运行会先构建，比较慢。
- `make dev-local` 和 `make dev-start` 完全相同。
- Vite 会把 `/api`、`/files` 等请求转发到 `http://localhost:8080`。
- 停止：在终端 A、B 按 Ctrl+C。基础服务会继续运行。

**anydoc 解析引擎**：本机目前没有 anydoc 静态库，所以方式一启动的后端不包含这个引擎，设置页会显示它不可用。需要的话，先运行一次 `make anydoc-lib`（需要 Rust 工具链），之后 `make dev-app` 会自动启用它。方式二的镜像默认就包含 anydoc。

## 方式二：打包成镜像运行

如果方式一正在运行，先在终端 A、B 按 Ctrl+C，空出 8080 和 5173 端口。然后运行：

```bash
make dev-docker DEV_ARGS=--no-langfuse
```

- 这条命令会用当前源码构建 `weknora-app:dev-local` 和 `weknora-ui:dev-local`，和基础服务一起启动，等所有服务的健康检查通过后才结束。
- 前端容器用 NGINX 提供构建好的页面，并把 API 请求转发给 app 容器。
- 改了代码：再运行一次同样的命令。它会利用构建缓存，只重建有变化的部分。第一次构建要编译 Go 和 Rust（anydoc），比较慢。
- 为了防止 Docker 虚拟机内存不够，构建 app 镜像时默认只同时编译 2 个 Go 包。内存充足的话，可以在 `.env` 里把 `GO_BUILD_PARALLELISM` 调大。
- 宿主机端口可以通过 `.env` 里的 `DEV_FRONTEND_PORT`、`DEV_APP_PORT` 修改。

## 切换与停止

| 想做的事 | 命令 |
| --- | --- |
| 方式一 → 方式二 | 在终端 A、B 按 Ctrl+C，再运行 `make dev-docker DEV_ARGS=--no-langfuse` |
| 方式二 → 方式一 | 运行 `make dev-local DEV_ARGS=--no-langfuse`（停掉 app 和前端容器，保留基础服务），再运行 `make dev-app`、`make dev-frontend` |
| 查看容器状态 | `make dev-status` |
| 查看容器日志 | `make dev-logs` |
| 停掉所有开发容器 | `make dev-stop` |

切换方式不会丢数据。

`make dev-stop` 会把 `WeKnora-neo4j` 也一起删掉，但数据卷会保留。下次开发前，先运行 `docker compose --profile neo4j up -d neo4j`。

## 常见问题

- **8080 或 5173 端口被占用**：运行 `lsof -i :8080` 查看占用端口的进程。常见原因是另一种方式还没停，或者正式环境（`docker compose up`）还在运行。开发环境和正式环境不能同时运行，详见 `CUSTOM_DEV.md`。
- **后端连不上 Neo4j**：`WeKnora-neo4j` 没在运行，按上文 [Neo4j](#neo4j) 启动它。
- **`make help` 里没有 `dev-docker` 等命令**：这些是定制命令，`make help` 显示的是官方帮助。全部选项见 `./scripts/dev-custom.sh help`。
- **一条 make 命令里不要混入其他目标**：比如 `make dev-stop build` 会整体交给官方 `Makefile` 处理，其中的 `dev-stop` 也会变成官方版本。分开运行就没问题。
