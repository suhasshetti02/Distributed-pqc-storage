# Distributed PQC Storage — Distributed Storage Fabric Prototype

A Go-based distributed file-storage prototype that combines peer-to-peer TCP communication, Reed–Solomon erasure coding, AES-GCM encryption, a SQLite metadata store, and an ML-KEM / Kyber-768 key-establishment exchange.

> **Project status:** Experimental prototype. Do not interpret the repository's “production-ready” badge or benchmark figures as an independent security or performance certification. The current default cluster launcher starts three local file servers, while the code's peer bootstrap logic is written around a four-peer target. Review the deployment topology before assuming a five-node cluster is being run.

## Implemented components

- HTTP gateway with login, file upload/download routes, and cluster-state dashboard.
- TCP peer transport and message handling.
- File storage with content-addressed identifiers derived from SHA-256.
- Reed–Solomon erasure coding helpers for shard creation and reconstruction.
- AES-GCM encryption / decryption helpers.
- CIRCL's Kyber-768 KEM API used for post-quantum shared-secret establishment.
- SQLite-backed metadata store.
- Periodic self-healing / shard-audit workflow and metadata gossip.
- Docker Compose configuration for local services.

## Technology

- Go (module declares Go 1.24).
- TCP networking and custom message encoding.
- CIRCL Kyber-768 KEM implementation.
- AES-GCM and SHA-256.
- Reed–Solomon library.
- SQLite through `modernc.org/sqlite`.
- Docker / Docker Compose.

## Architecture overview

```text
Client
  │ HTTP
  ▼
API gateway
  │
  ├── authentication / file metadata
  └── file server
        ├── peer TCP transport
        ├── KEM-based shared-secret exchange
        ├── AES-GCM protected shard messages
        ├── Reed–Solomon shard distribution / recovery
        ├── local disk storage
        └── SQLite metadata + periodic audit / gossip
```

The repository contains the gateway in `api_server.go`, peer file-server logic in `server.go`, storage in `store.go`, cryptographic helpers in `crypto.go`, and KEM exchange code under `p2p/`.

## Run locally

Requirements: Go 1.24-compatible toolchain and Docker / Docker Compose for the container setup.

```bash
go mod download
go test ./...
go run . -mode=cluster
```

The default API listener is `:8080`. The default cluster branch in `main.go` creates three file servers bound to ports 3000, 7000, and 5000. For a single node, use the `-mode=single`, `-node-id`, `-node-addr`, and `-bootstrap` flags. See `DOCS/OPERATIONS_GUIDE.md` and `docker-compose.yml` for deployment details.

Set `DFS_MASTER_KEY` to the expected 32-byte key value for repeatable encryption across restarts. Without a valid configured key, the current code generates a temporary random key for a local demo; data encrypted with that temporary key may not be decryptable after restart. Set `DFS_API_KEY` to replace the development fallback used for API JWT configuration.

## Repository documentation

- `DOCS/ENGINEERING_DESIGN.md` — design explanation.
- `DOCS/OPERATIONS_GUIDE.md` — operational instructions.
- `docker-compose.yml` — service topology.
- `go.mod` — Go dependencies.

## Security and correctness limitations

- The KEM exchange in `p2p/handshake.go` establishes a shared secret but does not, by itself, authenticate the remote peer's identity. Do not describe it as mutually authenticated zero-trust transport without an additional identity/signature trust mechanism.
- The handshake implementation should be reviewed for message framing, maximum node-ID length, protocol versioning, downgrade resistance, and peer authentication.
- The default encryption key fallback is random and temporary. Configure a stable secret key securely for persistent data.
- The API code includes a development fallback API key; production deployment must require strong configured secrets and enforce authentication on all sensitive endpoints.
- The default local cluster launcher starts three servers, not an automatically provisioned five-node cluster.
- The repository's performance claims should be treated as unverified unless the referenced benchmark scripts, workload, raw outputs, and environment are present and reproducible. The current tree does not include a complete automated benchmark suite.
- No security audit or formal cryptographic review is included.

## License

See repository license and dependency licenses.
