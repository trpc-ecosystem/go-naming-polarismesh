# Polaris Go 云端 E2E 打包与验证

[English](README.md)

该目录用于构建远端部署包，在云端测试机验证 naming 插件和通用注册
示例。部署包携带预编译示例二进制，远端不需要安装 Go，也不依赖 Go module
代理。

```bash
# 本地：构建 Linux amd64 部署包
./integration/cloud/build-package.sh

# 云端：校验、解压并只运行通用注册用例
sha256sum -c go-naming-polarismesh-cloud-*.sha256
tar xzf go-naming-polarismesh-cloud-*.tar.gz
cd go-naming-polarismesh-cloud-*
./run-e2e.sh --server <polaris-host>:8091 --only registry

# 云端：打包本次运行日志
./pack-logs.sh --run-dir cloud-runs/<run-id>

# 本地：安全导入并汇总回传日志
./integration/cloud/import-logs.sh <run-id>-logs.tar.gz
```

## 构建部署包

构建部署包需要 Go 1.21 或更高版本；正式发布包应使用 Go 1.26.11 或更高的
安全补丁工具链。

```bash
./integration/cloud/build-package.sh
./integration/cloud/build-package.sh --goos linux --goarch arm64
./examples/cloud-package.sh
```

默认目标为 Linux amd64。产物写入 `integration/cloud/dist/`：

```text
go-naming-polarismesh-cloud-<version>-<commit>-<time>-<os>-<arch>.tar.gz
go-naming-polarismesh-cloud-<version>-<commit>-<time>-<os>-<arch>.tar.gz.sha256
```

`integration/cloud/.build/` 和 `integration/cloud/dist/` 均为生成目录，已被
Git 忽略。需要清理时执行：

```bash
./integration/cloud/build-package.sh --clean
```

该命令会删除 `dist/` 中全部已生成的部署包及校验文件，确认不再需要后再执行。
如果使用了自定义的 `--build-root` 或 `--output-dir`，需把它们写在 `--clean`
之前。

部署包包含：

- 源码、examples、YAML 和 proto 资源；
- HelloWorld、共享 provider/client、multiContext、通用 metadata 快照
  和多 endpoint 注册的 Linux 预编译二进制；
- `run-e2e.sh`、`pack-logs.sh`、`import-logs.sh` 和
  `summarize-results.sh`；
- `manifest.json` 和包内 `SHA256SUMS`。

`manifest.json` 记录版本、Git commit、工作区是否 dirty、目标 OS/架构和构建
时间。构建和打包阶段不会写入 Polaris 地址或 token。

## 云端环境要求

远端测试机需要：

- 与部署包目标架构一致的 Linux；
- `bash`、`curl`、`jq`、`tar`、`ps` 和 GNU 兼容的 `timeout`；
- 可访问 Polaris gRPC 和 OpenAPI 端口。

示例程序已预编译，因此远端不需要安装 Go。

## 运行

运行全部示例：

```bash
./run-e2e.sh --server <polaris-host>:8091
```

只运行通用注册验证：

```bash
./run-e2e.sh --server <polaris-host>:8091 --only registry
```

`registry` 会展开为：

- `registry/metadata`：host/port/version/protocol 原子快照、metadata 透传、
  TTL、自动心跳、provider 失败和同地址覆盖；
- `registry/transports`：普通多 endpoint 身份、普通服务类型、反注册、
  稳定 ID 和 revision 变化。

指定多个场景：

```bash
./run-e2e.sh \
  --server <polaris-host>:8091 \
  --only helloworld,registry/metadata,registry/transports,route/rule
```

常用参数：

| 参数 | 说明 |
| --- | --- |
| `--server HOST[:PORT]` | Polaris 地址；省略端口时默认 `8091` |
| `--grpc-port PORT` | 覆盖 Polaris gRPC 端口 |
| `--http-port PORT` | Polaris OpenAPI 端口，默认 `8090` |
| `--namespace NAME` | E2E 命名空间，默认 `go-naming-polarismesh-e2e` |
| `--only LIST` | 逗号分隔的场景，或 `registry` |
| `--timeout SECONDS` | 单场景超时，默认 `600` 秒 |
| `--run-id ID` | 指定运行目录名 |
| `--runs-root DIR` | 运行目录根路径，默认 `cloud-runs` |
| `--no-archive` | 不自动生成日志压缩包 |

token 只从环境变量继承：

```bash
POLARIS_TOKEN=<optional-token> \
./run-e2e.sh --server <polaris-host>:8091 --only registry
```

运行记录只保存 token 是否设置，不保存 token 内容。

## 结果目录

每次运行写入：

```text
cloud-runs/<run-id>/
├── manifest.json
├── environment.txt
├── run.log
├── results.tsv
├── analysis.txt
├── logs/
└── runtime-records/
```

单个场景失败后仍会继续执行后续场景，最终结果表包含全部选中用例。任一场景
失败时脚本返回非零。

成功运行的进程、日志和远端服务默认保留供排查。清理需要显式执行：

```bash
./examples/cleanup.sh
POLARIS_TOKEN=<optional-token> \
./examples/cleanup.sh \
  --delete-service \
  --polaris-server <polaris-host> \
  --polaris-grpc-port 8091
```

## 日志回传

`run-e2e.sh` 默认生成：

```text
cloud-runs/<run-id>-logs.tar.gz
cloud-runs/<run-id>-logs.tar.gz.sha256
```

也可以手工打包已有运行：

```bash
./pack-logs.sh --run-dir cloud-runs/<run-id>
./pack-logs.sh --all
```

下载并导入本地：

```bash
scp <user>@<cloud-host>:~/cloud-runs/<run-id>-logs.tar.gz .
scp <user>@<cloud-host>:~/cloud-runs/<run-id>-logs.tar.gz.sha256 .
./integration/cloud/import-logs.sh ./<run-id>-logs.tar.gz
```

导入脚本会验证同级 `.sha256`，拒绝绝对路径、目录穿越和归档链接，并刷新
`analysis.txt`。

建议排查顺序：

1. `analysis.txt`
2. `results.tsv`
3. `logs/<scenario>.log`
4. 保留的 `logs/<scenario>/`
5. `polaris/log/` 和 `polaris/backup/`

## 安全要求

- 禁止上传 token、`.git`、缓存和历史运行目录。
- Polaris 地址只在运行时传入，不写入部署包默认值。
- 日志可能包含内网地址、实例 ID 和请求 metadata，回传和保存应符合部署环境
  的数据管理要求。
