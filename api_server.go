package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/reedsolomon"
)

type APIServer struct {
	listenAddr string
	fs         *FileServer
	meta       *MetadataStore
}

func NewAPIServer(listenAddr string, fs *FileServer, meta *MetadataStore) *APIServer {
	return &APIServer{
		listenAddr: listenAddr,
		fs:         fs,
		meta:       meta,
	}
}

func (s *APIServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/v1/auth/login", s.handleLogin)
	mux.HandleFunc("/v1/files", s.auth("writer")(s.handleFiles))
	mux.HandleFunc("/v1/files/", s.auth("reader", "writer")(s.handleFileByKey))
	mux.HandleFunc("/v1/cluster/state", s.handleClusterState)

	fmt.Printf("http gateway listening on %s\n", s.listenAddr)
	return http.ListenAndServe(s.listenAddr, mux)
}

func (s *APIServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 1. DYNAMIC IP DETECTION
	internalIP := "127.0.0.1"
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				internalIP = ipnet.IP.String()
				break
			}
		}
	}

	// 2. FETCH CLUSTER ANALYTICS
	files, _ := s.meta.List(100)
	peerCount := len(s.fs.Peers())

	totalActualSize := int64(0)
	uniqueCIDs := make(map[string]int64)
	for _, f := range files {
		totalActualSize += f.OriginalSize
		uniqueCIDs[f.CID] = f.OriginalSize
	}

	totalStoredSize := int64(0)
	for _, size := range uniqueCIDs {
		totalStoredSize += size
	}
	savings := totalActualSize - totalStoredSize
	if savings < 0 {
		savings = 0
	}

	// 3. GENERATE UNIFIED FILE AUDIT
	fileRows := ""
	for _, f := range files {
		health := "HEALTHY"
		missing := 0
		for _, sm := range f.Shards {
			found := false
			if sm.NodeID == s.fs.ID {
				found = true
			}
			for _, pid := range s.fs.Peers() {
				if pid == sm.NodeID {
					found = true
				}
			}
			if !found {
				missing++
			}
		}

		statusClass := "stat-up"
		if missing > 2 {
			health = "CRITICAL"
			statusClass = "stat-danger"
		} else if missing > 0 {
			health = "DEGRADED"
			statusClass = "stat-warn"
		}

		fileRows += fmt.Sprintf(`
			<div class="shard-item">
				<div style="flex: 2">
					<div style="font-weight:bold">%s</div>
					<div style="font-size:10px; color:var(--dim); font-family:monospace">%s</div>
				</div>
				<div style="flex: 1; text-align:center">%d MB</div>
				<div class="%s" style="flex: 1; text-align:right; font-weight:bold">%s</div>
			</div>`, f.Key, f.CID[:16]+"...", f.OriginalSize/1024/1024, statusClass, health)
	}

	if fileRows == "" {
		fileRows = `<div class="shard-item" style="color:var(--dim); justify-content:center">No files detected in the fabric.</div>`
	}

	// 4. GENERATE MESH HEATMAP
	heatmap := ""
	nodeShardCount := make(map[string]int)
	for _, f := range files {
		for _, sm := range f.Shards {
			nodeShardCount[sm.NodeID]++
		}
	}
	
	allNodeIDs := []string{s.fs.ID}
	for id := range s.fs.peers { allNodeIDs = append(allNodeIDs, id) }
	
	for _, id := range allNodeIDs {
		count := nodeShardCount[id]
		percent := (float64(count) / 10.0) * 100.0 // Assuming max 10 shards for demo
		if percent > 100 { percent = 100 }
		
		heatmap += fmt.Sprintf(`
			<div style="margin-bottom:15px">
				<div style="display:flex; justify-content:space-between; font-size:12px; margin-bottom:5px">
					<span>%s</span>
					<span>%d Shards</span>
				</div>
				<div style="height:6px; background:var(--border); border-radius:3px; overflow:hidden">
					<div style="width:%f%%; height:100%%; background:var(--primary)"></div>
				</div>
			</div>`, id, count, percent)
	}

	// 5. EXTRACT PORT
	addrParts := strings.Split(s.fs.Transport.Addr(), ":")
	port := "3000"
	if len(addrParts) > 1 {
		port = addrParts[len(addrParts)-1]
	}

	// 6. HEALTH LOGIC
	status := "ONLINE"
	statusClass := "stat-up"
	if peerCount < 4 {
		status = "DEGRADED"
		statusClass = "stat-warn"
	}
	if peerCount < 2 {
		status = "CRITICAL"
		statusClass = "stat-danger"
	}

	const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>NPS-DSS Node Console</title>
    <style>
        :root { --primary: #00f2fe; --bg: #0a0b10; --card: #15171e; --border: #242731; --dim: #8b949e; --success: #00ff88; --warn: #ffcc00; --danger: #ff4d4d; }
        body { background: var(--bg); color: white; font-family: 'Inter', sans-serif; margin: 0; padding: 40px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { display: flex; justify-content: space-between; align-items: flex-end; border-bottom: 2px solid var(--primary); padding-bottom: 20px; margin-bottom: 40px; }
        .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 20px; margin-bottom: 40px; }
        .card { background: var(--card); border: 1px solid var(--border); padding: 25px; border-radius: 8px; position: relative; overflow: hidden; }
        .card::before { content: ""; position: absolute; top:0; left:0; width: 4px; height: 100%%; background: var(--primary); opacity: 0.5; }
        .label { color: var(--dim); font-size: 11px; text-transform: uppercase; margin-bottom: 8px; }
        .value { font-size: 20px; font-weight: bold; }
        .meta-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px; }
        .meta-item { background: rgba(255,255,255,0.03); padding: 10px; border-radius: 4px; font-size: 12px; }
        .stat-up { color: var(--success); }
        .stat-warn { color: var(--warn); }
        .stat-danger { color: var(--danger); }
        h2 { font-size: 16px; margin-bottom: 20px; color: var(--dim); display: flex; align-items: center; gap: 10px; text-transform: uppercase; letter-spacing: 2px; }
        h2::after { content: ""; flex: 1; height: 1px; background: var(--border); }
        .audit-container { display: grid; grid-template-columns: 2.5fr 1fr; gap: 30px; }
        .shard-list { background: var(--card); border: 1px solid var(--border); border-radius: 4px; }
        .shard-item { padding: 15px 25px; border-bottom: 1px solid var(--border); display: flex; justify-content: space-between; align-items: center; }
        .shard-item:last-child { border: 0; }
        .tag { font-size: 10px; padding: 2px 8px; background: rgba(0,242,254,0.1); border: 1px solid var(--primary); border-radius: 10px; color: var(--primary); }
        .savings { font-size: 12px; color: var(--success); margin-top: 5px; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div>
                <h1 style="margin:0; font-size: 32px; letter-spacing: -1px;">COMMAND_CENTER: <span style="color:var(--primary)">%s</span></h1>
                <p style="margin:5px 0 0 0; color: var(--dim); font-size: 14px;">Cluster Fabric Protocol v1.0 | Mode: Distributed Mesh</p>
            </div>
            <div style="text-align:right">
                <div class="%s" style="font-weight:bold">%s</div>
                <div style="font-size:12px; color:var(--dim)">Network Quorum: Active</div>
            </div>
        </div>

        <h2>FABRIC_METRICS</h2>
        <div class="grid">
            <div class="card">
                <div class="label">System Status</div>
                <div class="value %s">%s</div>
                <div class="meta-grid">
                    <div class="meta-item">Internal IP: %s</div>
                    <div class="meta-item">Health: %d%%</div>
                </div>
            </div>
            <div class="card">
                <div class="label">Cluster Discovery</div>
                <div class="value stat-up">CONNECTED</div>
                <div class="meta-grid">
                    <div class="meta-item">Peers: %d</div>
                    <div class="meta-item">Port: %s</div>
                </div>
            </div>
            <div class="card">
                <div class="label">Storage Efficiency</div>
                <div class="value">CAS / Deduplication</div>
                <div class="savings">Saved: %d MB Total</div>
            </div>
            <div class="card">
                <div class="label">Cryptography</div>
                <div class="value" style="color:var(--success)">PQC SECURED</div>
                <div class="meta-grid">
                    <div class="meta-item">AES-256-GCM</div>
                    <div class="meta-item">ML-KEM-768</div>
                </div>
            </div>
        </div>

        <div class="audit-container">
            <div>
                <h2>MESH_STORAGE_AUDIT</h2>
                <div class="shard-list">
                    %s
                </div>
            </div>
            <div>
                <h2>FABRIC_HEATMAP</h2>
                <div class="card" style="padding:20px">
                    %s
                </div>
            </div>
        </div>
    </div>
</body>
</html>
	`
	health := (peerCount + 1) * 20
	fmt.Fprintf(w, htmlTemplate, s.fs.ID, statusClass, status, statusClass, status, internalIP, health, peerCount, port, savings/1024/1024, fileRows, heatmap)
}

func (s *APIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	var role string
	if username == "admin" && password == "adminpass" {
		role = "writer"
	} else if username == "reader" && password == "readerpass" {
		role = "reader"
	} else {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	token, err := GenerateJWT(username, role)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "role": role})
}

func (s *APIServer) auth(allowedRoles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			role, err := getRoleFromRequest(r)
			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
				return
			}

			allowed := false
			for _, ar := range allowedRoles {
				if ar == role {
					allowed = true
					break
				}
			}

			if !allowed {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}

			next(w, r)
		}
	}
}

func (s *APIServer) handleFiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleUpload(w, r)
	case http.MethodGet:
		s.handleList(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *APIServer) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := s.meta.List(100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *APIServer) handleFileByKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/v1/files/")
	log.Printf("[API] %s %s (Key: %s)", r.Method, r.URL.Path, key)
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file key"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, err := s.meta.GetByKey(key)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}

		enc, err := reedsolomon.New(3, 2)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to init erasure coding"})
			return
		}

		shards := make([][]byte, 5)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, sm := range rec.Shards {
			wg.Add(1)
			go func(sm ShardMeta) {
				defer wg.Done()
				data, err := s.fs.GetShard(sm.NodeID, sm.CID)
				if err != nil {
					log.Printf("[Recovery] skipping shard %d from node %s: %v", sm.Index, sm.NodeID, err)
					return
				}
				mu.Lock()
				shards[sm.Index] = data
				mu.Unlock()
			}(sm)
		}
		wg.Wait()

		// 3. RECONSTRUCT IF NECESSARY
		// Verify if we have at least 3 shards (the minimum for 3+2)
		have := 0
		for _, s := range shards {
			if s != nil {
				have++
			}
		}

		if have < 3 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("not enough shards available (have %d, need 3)", have)})
			return
		}

		// If we have between 3 and 4 shards, we must reconstruct the missing ones.
		// reedsolomon.Reconstruct will allocate and fill the nil shards.
		if have < 5 {
			if err := enc.Reconstruct(shards); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "erasure coding reconstruction failed: " + err.Error()})
				return
			}
		}

		var out bytes.Buffer
		if err := enc.Join(&out, shards, int(rec.OriginalSize)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to join shards: " + err.Error()})
			return
		}

		w.Header().Set("Content-Disposition", "attachment; filename=\""+key+"\"")
		w.Header().Set("X-Content-CID", rec.CID)
		w.Header().Set("Content-Length", strconv.FormatInt(rec.OriginalSize, 10))
		w.WriteHeader(http.StatusOK)

		_, _ = io.Copy(w, &out)
	case http.MethodDelete:
		// Not implemented in this version
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *APIServer) handleUpload(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = strings.TrimSpace(r.Header.Get("X-File-Key"))
	}
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide key in query or X-File-Key"})
		return
	}

	var content io.Reader
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to get file from form: " + err.Error()})
			return
		}
		defer file.Close()
		content = file
	} else {
		content = r.Body
		defer r.Body.Close()
	}

	data, err := io.ReadAll(content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read upload data: " + err.Error()})
		return
	}

	originalSize := int64(len(data))

	overallHasher := sha256.New()
	overallHasher.Write(data)
	overallCID := hex.EncodeToString(overallHasher.Sum(nil))

	// DEDUPLICATION CHECK
	if existing, err := s.meta.GetByCID(overallCID); err == nil && existing != nil {
		log.Printf("[Deduplication] File with CID %s already exists. Linking to existing shards.", overallCID)
		_ = s.meta.UpsertFile(key, overallCID, originalSize, existing.Shards)
		// GOSSIP: Even if deduplicated, we must tell others about the new KEY
		go s.fs.BroadcastMetadata(FileRecord{
			Key:          key,
			CID:          overallCID,
			OriginalSize: originalSize,
			Shards:       existing.Shards,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"key":     key,
			"cid":     overallCID,
			"message": "file already exists (deduplicated)",
			"shards":  len(existing.Shards),
		})
		return
	}

	padLen := 3 - (len(data) % 3)
	if padLen == 3 {
		padLen = 0
	}
	if padLen > 0 {
		data = append(data, make([]byte, padLen)...)
	}

	enc, err := reedsolomon.New(3, 2)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to init erasure coding: " + err.Error()})
		return
	}

	shards, err := enc.Split(data)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to split data: " + err.Error()})
		return
	}

	if err := enc.Encode(shards); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encode parity: " + err.Error()})
		return
	}

	// QUORUM GUARD: Wait for the mesh to be fully converged (5 nodes total)
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		peers := s.fs.Peers()
		availableNodes := append([]string{s.fs.ID}, peers...)
		if len(availableNodes) >= 5 {
			break
		}
		if i == maxRetries-1 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": fmt.Sprintf("mesh quorum not reached (found %d/5 nodes)", len(availableNodes))})
			return
		}
		log.Printf("[Quorum Guard] Waiting for mesh convergence... (%d/5 nodes found)", len(availableNodes))
		time.Sleep(time.Second * 1)
	}

	peers := s.fs.Peers()
	availableNodes := append([]string{s.fs.ID}, peers...)

	var shardManifest []ShardMeta
	for i, shardData := range shards {
		cid := hashKey(string(shardData))
		nodeID := availableNodes[i%len(availableNodes)]

		// Encrypt shard at the gateway level before distribution
		encryptedBlob, err := EncryptData(s.fs.EncKey, shardData)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to encrypt shard: " + err.Error()})
			return
		}

		if err := s.fs.StoreShard(nodeID, cid, bytes.NewReader(encryptedBlob)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store shard: " + err.Error()})
			return
		}

		shardManifest = append(shardManifest, ShardMeta{
			Index:  i,
			CID:    cid,
			NodeID: nodeID,
		})
	}

	if err := s.meta.UpsertFile(key, overallCID, originalSize, shardManifest); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save manifest: " + err.Error()})
		return
	}

	// GOSSIP: Tell everyone about this new file
	rec := FileRecord{
		Key:          key,
		CID:          overallCID,
		OriginalSize: originalSize,
		Shards:       shardManifest,
	}
	go s.fs.BroadcastMetadata(rec)

	writeJSON(w, http.StatusCreated, map[string]any{"key": key, "cid": overallCID, "size": originalSize, "shards": len(shardManifest)})
}

func (s *APIServer) handleClusterState(w http.ResponseWriter, r *http.Request) {
	type NodeState struct {
		ID     string `json:"id"`
		IP     string `json:"ip"`
		Status string `json:"status"`
		Shards int    `json:"shards"`
	}

	internalIP := "localhost"
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				internalIP = ipnet.IP.String()
				break
			}
		}
	}

	states := []NodeState{}
	states = append(states, NodeState{
		ID:     s.fs.ID,
		IP:     internalIP,
		Status: "UP",
		Shards: 0,
	})

	for id, peer := range s.fs.peers {
		states = append(states, NodeState{
			ID:     id,
			IP:     peer.RemoteAddr().String(),
			Status: "UP",
			Shards: 0,
		})
	}

	writeJSON(w, http.StatusOK, states)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status >= 400 {
		log.Printf("[API ERROR] Status: %d Payload: %+v", status, payload)
	}
	_ = json.NewEncoder(w).Encode(payload)
}
