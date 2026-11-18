# Polaris Go Cloud E2E Packaging

[Chinese](README.zh_CN.md)

This directory builds a deployment package for validating the naming plugin
and generic registration examples on a remote host. The package contains precompiled
example binaries, so the remote host does not need Go or access to a Go module
proxy.

```bash
# Local: build a Linux amd64 package
./integration/cloud/build-package.sh

# Cloud: verify the checksum, extract and run registry cases
sha256sum -c go-naming-polarismesh-cloud-*.sha256
tar xzf go-naming-polarismesh-cloud-*.tar.gz
cd go-naming-polarismesh-cloud-*
./run-e2e.sh --server <polaris-host>:8091 --only registry

# Cloud: create a checksummed log archive
./pack-logs.sh --run-dir cloud-runs/<run-id>

# Local: import and summarize returned logs
./integration/cloud/import-logs.sh <run-id>-logs.tar.gz
```

## Build

Use Go 1.21 or newer to build the package. Release packages should be produced
with Go 1.26.11 or a newer patched toolchain.

```bash
./integration/cloud/build-package.sh
./integration/cloud/build-package.sh --goos linux --goarch arm64
./examples/cloud-package.sh
```

The default target is Linux amd64. The output directory is
`integration/cloud/dist/` and contains:

```text
go-naming-polarismesh-cloud-<version>-<commit>-<time>-<os>-<arch>.tar.gz
go-naming-polarismesh-cloud-<version>-<commit>-<time>-<os>-<arch>.tar.gz.sha256
```

Both `integration/cloud/.build/` and `integration/cloud/dist/` are generated
and git-ignored. To remove them explicitly:

```bash
./integration/cloud/build-package.sh --clean
```

This deletes all packaged archives and checksums in `dist/`, so only run it
when no generated package is still needed. With custom `--build-root` or
`--output-dir` paths, put those options before `--clean`.

The package includes:

- source, examples and all YAML/proto assets;
- Linux binaries for HelloWorld, shared provider/client, multi-context,
  opaque metadata snapshots and multi-endpoint registration;
- `run-e2e.sh`, `pack-logs.sh`, `import-logs.sh` and `summarize-results.sh`;
- `manifest.json` and an internal `SHA256SUMS`.

`manifest.json` records the package version, Git commit, dirty-worktree state,
target OS/architecture and build time. Tokens and Polaris addresses are not
written during packaging.

## Remote Requirements

The remote host needs:

- Linux with the package target architecture;
- `bash`, `curl`, `jq`, `tar`, `ps` and GNU-compatible `timeout`;
- network access to the Polaris gRPC and OpenAPI ports.

Go is not required because all example programs are precompiled.

## Run

Run all supported examples:

```bash
./run-e2e.sh --server <polaris-host>:8091
```

Run only the generic registration validation:

```bash
./run-e2e.sh --server <polaris-host>:8091 --only registry
```

`registry` expands to:

- `registry/metadata`: atomic host/port/version/protocol snapshots, opaque
  metadata, TTL, automatic heartbeat, provider failure and replacement;
- `registry/transports`: ordinary multi-endpoint identity, microservice type,
  deregistration, stable IDs and revision changes.

Run an explicit scenario list:

```bash
./run-e2e.sh \
  --server <polaris-host>:8091 \
  --only helloworld,registry/metadata,registry/transports,route/rule
```

Common options:

| Option | Description |
| --- | --- |
| `--server HOST[:PORT]` | Polaris host; omitted port defaults to `8091` |
| `--grpc-port PORT` | Override the Polaris gRPC port |
| `--http-port PORT` | Polaris OpenAPI port, default `8090` |
| `--namespace NAME` | E2E namespace, default `go-naming-polarismesh-e2e` |
| `--only LIST` | Comma-separated scenarios or `registry` |
| `--timeout SECONDS` | Per-scenario timeout, default `600` |
| `--run-id ID` | Stable run directory name |
| `--runs-root DIR` | Run output root, default `cloud-runs` |
| `--no-archive` | Skip automatic log archive creation |

Credentials are inherited only from environment variables:

```bash
POLARIS_TOKEN=<optional-token> \
./run-e2e.sh --server <polaris-host>:8091 --only registry
```

The wrapper records whether a token is set, but never writes its value.

## Results

Each run is written below:

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

Scenario execution continues after a failure so the final result table shows
every selected case. The process exits non-zero if any scenario fails.

Successful processes, logs and remote services are retained by default for
inspection. Cleanup is explicit:

```bash
./examples/cleanup.sh
POLARIS_TOKEN=<optional-token> \
./examples/cleanup.sh \
  --delete-service \
  --polaris-server <polaris-host> \
  --polaris-grpc-port 8091
```

## Log Transfer

`run-e2e.sh` normally creates:

```text
cloud-runs/<run-id>-logs.tar.gz
cloud-runs/<run-id>-logs.tar.gz.sha256
```

Pack an existing run manually:

```bash
./pack-logs.sh --run-dir cloud-runs/<run-id>
./pack-logs.sh --all
```

Download and import the archive locally:

```bash
scp <user>@<cloud-host>:~/cloud-runs/<run-id>-logs.tar.gz .
scp <user>@<cloud-host>:~/cloud-runs/<run-id>-logs.tar.gz.sha256 .
./integration/cloud/import-logs.sh ./<run-id>-logs.tar.gz
```

The importer verifies the adjacent checksum, rejects absolute/traversal paths
and archive links, and refreshes `analysis.txt`.

Inspect in this order:

1. `analysis.txt`
2. `results.tsv`
3. `logs/<scenario>.log`
4. retained `logs/<scenario>/`
5. `polaris/log/` and `polaris/backup/`

## Security

- Do not upload tokens, `.git`, caches or previous run directories.
- Polaris addresses are runtime arguments, not package defaults.
- Logs can contain internal hosts, instance IDs and request metadata; handle
  returned archives according to the deployment environment's data policy.
