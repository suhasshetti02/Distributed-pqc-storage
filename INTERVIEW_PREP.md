# Distributed-pqc-storage: Interview Preparation Guide

This document is a comprehensive guide to help you explain, defend, and showcase the **Distributed-pqc-storage (Post-Quantum Secure Distributed Storage Fabric)** project in a technical interview setting. 

---

## 1. Project Overview & How It Works

**Distributed-pqc-storage** is a zero-trust, self-healing distributed storage system designed to withstand the next generation of cryptographic threats (quantum computing) and infrastructure failures. 

### How It Works (The Workflow)
1. **Upload & Encryption**: A user uploads a file to the Gateway Node via HTTP. The Gateway encrypts the payload symmetrically using **AES-GCM**.
2. **Content-Aware Storage (CAS)**: The system checks if the file (or its exact content) already exists using cryptographic hashing, deduplicating the data to save storage costs.
3. **Erasure Coding (Sharding)**: The encrypted file is mathematically split using a **Reed-Solomon (3+2)** scheme into 3 data shards and 2 parity shards. 
4. **Secure P2P Distribution**: The shards are distributed across 5 storage nodes. The nodes authenticate and establish secure communication channels using **ML-KEM-768** (Post-Quantum Cryptography).
5. **Autonomous Self-Healing**: A background gossip/audit protocol monitors the mesh. If 1 or 2 nodes fail, the system detects the missing shards, mathematically reconstructs them from the surviving nodes using the parity data, and redistributes them to healthy nodes to restore 100% redundancy.

### Technology Stack
* **Language**: Go (Golang) 1.24 - chosen for its high-performance concurrency model and excellent networking primitives.
* **Post-Quantum Cryptography**: `github.com/cloudflare/circl` (Implementing ML-KEM-768 / Kyber for quantum-resistant handshakes).
* **Data Resilience / Sharding**: `github.com/klauspost/reedsolomon` for 3+2 erasure coding.
* **Metadata & Deduplication**: `modernc.org/sqlite` (CGO-free SQLite) for managing Content-Aware Storage metadata.
* **Infrastructure**: Docker & Docker Compose for local cluster simulation and deployment.
* **Authentication**: JWT (`golang-jwt/jwt`) for API client access.

### Strengths & Advantages
* **Future-Proof Security**: Protects against "Harvest Now, Decrypt Later" attacks by utilizing standardized Post-Quantum Cryptography (NIST ML-KEM).
* **High Availability & Fault Tolerance**: With 3+2 Reed-Solomon, the cluster survives the simultaneous catastrophic failure of any 2 nodes (40% of the network) with zero data loss.
* **Storage Efficiency**: Deduplication ensures identical files don't consume extra shards in the network.
* **Operational Autonomy**: The system self-heals without human intervention, reducing DevOps overhead.

---

## 2. How to Execute & Run the Project

During an interview, you can confidently explain how to run the project locally to demonstrate its capabilities.

### Prerequisites
* Docker & Docker Compose
* PowerShell (for running the audit/test suites)

### Execution Steps
1. **Spin up the Cluster**:
   ```bash
   docker-compose up -d --build
   ```
   *This starts the 5-node mesh network and the API Gateway.*

2. **Run the Audit/Verification Suite**:
   ```powershell
   .\test_system.ps1
   ```
   *This PowerShell script verifies the core features: it uploads a file, asserts deduplication, forcefully kills 2 Docker containers to simulate node failure, and proves the data can still be reconstructed.*

3. **Access the Dashboard**:
   Navigate to `http://localhost:8080` to interact with the API Gateway and monitor the mesh's real-time heatmap.

---

## 3. Explaining the Project using the STAR Method

When asked: *"Tell me about a complex distributed system you built,"* use this STAR response.

### **Situation (S)**
"I wanted to build a highly resilient distributed file system that addressed two major modern engineering concerns: the looming threat of quantum computers breaking current RSA/ECC encryption protocols ('Harvest Now, Decrypt Later' attacks), and the high operational cost of maintaining data integrity when hardware fails in a distributed environment."

### **Task (T)**
"My goal was to design and implement a zero-trust, peer-to-peer storage fabric from scratch in Go. It needed to securely distribute data across multiple nodes, survive the catastrophic failure of at least 40% of the network, and automatically repair itself without any manual DevOps intervention."

### **Action (A)**
"To achieve this, I architected **Distributed-pqc-storage**. 
1. I integrated **Cloudflare's CIRCL library** to implement **ML-KEM-768** for post-quantum secure peer-to-peer handshakes, securing node communication. 
2. Instead of simple data replication, which is expensive, I implemented **Reed-Solomon 3+2 Erasure Coding**. This split the data into 3 data blocks and 2 parity blocks, distributing them across a 5-node cluster.
3. I built a custom **TCP transport and Gossip protocol** to sync metadata across the network.
4. I engineered a background **Audit Engine** that continuously monitors node health. If it detects a node failure, it pulls the surviving shards, mathematically recalculates the missing ones in real-time, and pushes them to healthy nodes.
5. Finally, I added a **Content-Aware Storage (CAS)** layer backed by SQLite to deduplicate files at the gateway level."

### **Result (R)**
"The result is a production-ready, highly secure storage fabric. In my benchmark tests, the system achieved an encoding latency of under 6.7 seconds for 10MB payloads including the PQC handshakes. Most importantly, I successfully proved through automated chaos engineering that the cluster can lose 2 out of 5 nodes simultaneously and still reconstruct the data perfectly within 5 seconds, all while saving storage space through deterministic deduplication."

---

## 4. Probable Interview Questions & Answers

**Q1: Why did you choose Reed-Solomon Erasure Coding over simple Data Replication?**
> *Answer:* Simple replication (like storing 3 exact copies of a file) takes 300% storage overhead. With Reed-Solomon 3+2, we split the data into 3 chunks and add 2 parity chunks. The storage overhead is only 166% (5/3), yet we still achieve the ability to lose 2 distinct disks/nodes. It drastically reduces storage costs while maintaining high fault tolerance.

**Q2: How does the Post-Quantum Cryptography (ML-KEM) actually fit into the system?**
> *Answer:* Standard TLS uses elliptic curve cryptography (ECDH) for key exchange, which is vulnerable to Shor's algorithm on a quantum computer. I used ML-KEM-768 (Kyber), the NIST standard for lattice-based key encapsulation, for the peer-to-peer handshakes between the storage nodes. Once the symmetric key is securely encapsulated and exchanged, the actual payload is encrypted using AES-GCM for performance.

**Q3: How does the Content-Aware Storage (CAS) layer deduplicate files?**
> *Answer:* Before sharding, the Gateway hashes the incoming file's content (e.g., using SHA-256). This hash becomes the internal Content Identifier (CID). If user A and user B upload the exact same file under different names, the system recognizes the identical CID, points both users' metadata to the same underlying shards, and skips the network upload entirely for the second user. 

**Q4: Explain how the Self-Healing / Audit engine works under the hood.**
> *Answer:* The nodes use a background heartbeat/gossip mechanism to track active peers. The Audit Engine periodically cross-references the metadata database with the active nodes. If a node holding Shard #4 goes offline, the engine reads 3 of the remaining shards (e.g., Shards 1, 2, and 5), uses matrix inversion on the Galois field to recalculate Shard #4, and writes it to a newly spun-up or healthy node. 

**Q5: What were the hardest challenges you faced while building this in Go?**
> *Answer:* The biggest challenge was orchestrating the concurrent network streams during the Reed-Solomon reconstruction phase. Fetching shards from 3 different nodes over TCP required careful use of Go routines, `sync.WaitGroup`, and context timeouts to prevent deadlocks if a peer responded too slowly during the recovery process. I also had to optimize the memory allocations (using `io.Reader` and buffers) so the API gateway wouldn't OOM (Out Of Memory) when encrypting and sharding large files.

**Q6: What happens if the API Gateway goes down? Is it a single point of failure?**
> *Answer:* In the current local deployment, the gateway acts as the entry point. However, the system architecture allows *any* node to act as a gateway because the metadata and P2P routing are decentralized. In a true production environment, I would put a Load Balancer (like Nginx or HAProxy) in front of all 5 nodes, making the entire fabric stateless from the client's perspective.
