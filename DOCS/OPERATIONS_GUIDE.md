# Hydra-PQC: Operations & Deployment Guide

This guide provides technical instructions for deploying, managing, and testing the HydraStore Distributed Mesh.

## 1. Project Folder Structure

```text
Hydra-PQC/
├── README.md               # High-level overview & metrics
├── ENGINEERING_DESIGN.md   # Architectural & Cryptographic specs
├── OPERATIONS_GUIDE.md     # Setup and Command reference
├── main.go                 # Entry point (Flag parsing & init)
├── server.go               # Core Mesh Transport & Self-Healing
├── api_server.go           # Gateway & Web Command Center
├── metadata_store.go       # SQLite persistence for file manifests
├── crypto.go               # PQC (ML-KEM) & AES-256 implementation
├── store.go                # Physical shard storage management
├── auth.go                 # JWT-based Role-Based Access Control
├── p2p/                    # Low-level TCP Transport & Handshaking
├── scripts/                # Utility scripts
│   ├── test_system.ps1     # Automated end-to-end audit
│   └── benchmark.ps1       # High-precision performance suite
├── Dockerfile              # Containerization manifest
└── docker-compose.yml      # 5-node mesh orchestration
```

## 2. Deployment Commands

### Primary Cluster Launch
Launches 5 nodes (node1 to node5) in a shared virtual network.
```bash
docker-compose up -d --build
```

### Checking Cluster Logs
```bash
docker-compose logs -f
```

### Stopping the Cluster
```bash
docker-compose down
```

## 3. Operations & Testing

### Automated Deep Audit
This script performs a 5-phase test:
1.  **Reset**: Cleans the cluster state.
2.  **Upload**: Distributes test media across nodes.
3.  **Deduplication**: Verifies CAS efficiency.
4.  **Fault Injection**: Stops 2 nodes (40% of cluster).
5.  **Recovery**: Reconstructs data using surviving shards.
```powershell
.\test_system.ps1
```

### Performance Benchmarking
Generates the real-time metrics (latency, throughput) for the system.
```powershell
.\benchmark.ps1
```

## 4. Manual Gateway Interaction

The HTTP Gateway runs on `http://localhost:8080`.

### Uploading a File (via CURL)
```bash
curl -X POST -H "Authorization: Bearer <TOKEN>" \
     -F "file=@yourfile.txt" \
     "http://localhost:8080/v1/files?key=yourfile.txt"
```

### Downloading a File
```bash
curl -H "Authorization: Bearer <TOKEN>" \
     "http://localhost:8080/v1/files/yourfile.txt" \
     --output recovered.txt
```

## 5. Maintenance & Reset
To completely wipe the cluster state and start fresh:
1.  Stop the containers: `docker-compose down`
2.  Delete local data: `Remove-Item -Recurse node*_network, dfs_meta.db` (PowerShell)
3.  Clean docker volumes: `docker volume prune`

---
## Troubleshooting
*   **Gossip Convergence**: If metadata isn't appearing on all nodes, wait 5-10 seconds for the mesh to propagate.
*   **Quorum Errors**: Ensure at least 3 nodes are online; the system requires a 60% quorum for shard distribution.
