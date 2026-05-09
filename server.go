package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"dfs/p2p"
	"github.com/klauspost/reedsolomon"
)

type FileServerOpts struct {
	ID                string
	EncKey            []byte
	StorageRoot       string
	PathTransformFunc PathTransformFunc
	Transport         p2p.Transport
	BootstrapNodes    []string
	Meta              *MetadataStore
}

type FileServer struct {
	FileServerOpts

	peerLock sync.Mutex
	peers    map[string]p2p.Peer
	store    *Store
	quitch   chan struct{}
}

func NewFileServer(opts FileServerOpts) *FileServer {
	storeOpts := StoreOpts{
		Root:              opts.StorageRoot,
		PathTransformFunc: opts.PathTransformFunc,
	}

	return &FileServer{
		FileServerOpts: opts,
		store:          NewStore(storeOpts),
		quitch:         make(chan struct{}),
		peers:          make(map[string]p2p.Peer),
	}
}

func (s *FileServer) Peers() []string {
	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	res := []string{}
	for id := range s.peers {
		res = append(res, id)
	}
	return res
}

func (s *FileServer) Start() error {
	if err := s.Transport.ListenAndAccept(); err != nil {
		return err
	}

	s.bootstrapNetwork()
	go s.loop()

	return nil
}

func (s *FileServer) Stop() {
	close(s.quitch)
}

func (s *FileServer) bootstrapNetwork() {
	for _, addr := range s.BootstrapNodes {
		if len(addr) == 0 {
			continue
		}

		go func(addr string) {
			for {
				s.peerLock.Lock()
				count := len(s.peers)
				s.peerLock.Unlock()

				if count >= 4 {
					log.Printf("[%s] FULL MESH ESTABLISHED (4/4 peers). Ready.", s.ID)
					break
				}

				log.Printf("[%s] Mesh Discovery: %d/4 peers found. Retrying %s...", s.ID, count, addr)
				if err := s.Transport.Dial(addr); err != nil {
					log.Printf("dial error: %v", err)
				}
				time.Sleep(time.Second * 3)
			}
		}(addr)
	}
}

func (s *FileServer) OnPeer(p p2p.Peer) error {
	nodeID := p.RemoteNodeID()
	s.peerLock.Lock()
	defer s.peerLock.Unlock()

	s.peers[nodeID] = p
	log.Printf("[%s] connected to remote %s", s.ID, nodeID)

	return nil
}

func (s *FileServer) loop() {
	defer func() {
		log.Println("file server stopped due to error or quit action")
		s.Transport.Close()
	}()

	ticker := time.NewTicker(time.Second * 30)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.performSelfHealing()
		case rpc := <-s.Transport.Consume():
			var msg Message
			if err := gob.NewDecoder(bytes.NewReader(rpc.Payload)).Decode(&msg); err != nil {
				log.Printf("decode error: %v", err)
				continue
			}

			if err := s.handleMessage(rpc.From, &msg); err != nil {
				log.Printf("handle message error: %v", err)
			}

		case <-s.quitch:
			return
		}
	}
}

func (s *FileServer) handleMessage(from string, msg *Message) error {
	switch v := msg.Payload.(type) {
	case MessageStoreFile:
		return s.handleMessageStoreFile(from, v)
	case MessageGetFile:
		return s.handleMessageGetFile(from, v)
	case MessageGetFileResponse:
		respMu.Lock()
		ch, ok := respChannels[v.Key]
		respMu.Unlock()
		if ok {
			// Decrypt the remote shard before passing it back to recovery logic
			decrypted, err := decryptData(s.EncKey, v.Data)
			if err != nil {
				log.Printf("failed to decrypt remote shard %s: %v", v.Key, err)
				return err
			}
			ch <- decrypted
		}
	case MessageMetadataSync:
		// Check if we already have this specific key to prevent infinite gossip loops
		exists, _ := s.Meta.HasKey(v.Key)
		if exists {
			return nil
		}

		log.Printf("[%s] Received metadata sync for: %s. Gossiping further...", s.ID, v.Key)
		err := s.Meta.UpsertFile(v.Key, v.CID, v.OriginalSize, v.Shards)
		if err == nil {
			// RE-BROADCAST to peers (True Gossip)
			go s.BroadcastMetadata(FileRecord(v))
		}
		return err
	}
	return nil
}

func (s *FileServer) StoreShard(nodeID string, key string, r io.Reader) error {
	shardData, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	if nodeID == s.ID {
		log.Printf("[%s] Storing shard locally: %s", s.ID, key)
		_, err := s.store.Write(s.ID, key, bytes.NewReader(shardData))
		return err
	}

	s.peerLock.Lock()
	peer, ok := s.peers[nodeID]
	s.peerLock.Unlock()

	if !ok {
		return fmt.Errorf("node %s not found in mesh", nodeID)
	}

	log.Printf("[%s] Routing shard to remote node %s: %s", s.ID, nodeID, key)
	var buf bytes.Buffer
	msg := Message{
		Payload: MessageStoreFile{
			Key:  key,
			Data: shardData,
		},
	}
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	return sendControlMessage(peer, buf.Bytes())
}

func (s *FileServer) GetShard(nodeID string, key string) ([]byte, error) {
	if nodeID == s.ID {
		_, r, err := s.store.Read(s.ID, key)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		// Since it's local, we need to decrypt it before returning to match network behavior
		return decryptData(s.EncKey, data)
	}

	s.peerLock.Lock()
	peer, ok := s.peers[nodeID]
	s.peerLock.Unlock()

	if !ok {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	msg := Message{
		Payload: MessageGetFile{
			Key: key,
		},
	}

	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(msg); err != nil {
		return nil, err
	}

	if err := sendControlMessage(peer, buf.Bytes()); err != nil {
		return nil, err
	}

	return s.waitForResponse(key)
}

var (
	respChannels = make(map[string]chan []byte)
	respMu       sync.Mutex
)

func (s *FileServer) waitForResponse(key string) ([]byte, error) {
	ch := make(chan []byte, 1)
	respMu.Lock()
	respChannels[key] = ch
	respMu.Unlock()

	defer func() {
		respMu.Lock()
		delete(respChannels, key)
		respMu.Unlock()
	}()

	select {
	case data := <-ch:
		return data, nil
	case <-time.After(time.Second * 5):
		return nil, fmt.Errorf("timeout waiting for shard %s", key)
	}
}

func (s *FileServer) BroadcastMetadata(rec FileRecord) error {
	log.Printf("[%s] Gossiping metadata for %s to mesh...", s.ID, rec.Key)
	msg := Message{
		Payload: MessageMetadataSync(rec),
	}

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(msg); err != nil {
		return err
	}

	s.peerLock.Lock()
	defer s.peerLock.Unlock()
	for id, peer := range s.peers {
		if err := sendControlMessage(peer, buf.Bytes()); err != nil {
			log.Printf("failed to sync metadata with %s: %v", id, err)
		}
	}
	return nil
}

func (s *FileServer) handleMessageStoreFile(from string, msg MessageStoreFile) error {
	_, err := s.store.Write(s.ID, msg.Key, bytes.NewReader(msg.Data))
	return err
}

func (s *FileServer) handleMessageGetFile(from string, msg MessageGetFile) error {
	if !s.store.Has(s.ID, msg.Key) {
		return fmt.Errorf("file not found: %s", msg.Key)
	}

	_, r, err := s.store.Read(s.ID, msg.Key)
	if err != nil {
		return err
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	resp := Message{
		Payload: MessageGetFileResponse{
			Key:  msg.Key,
			Size: int64(len(data)),
			Data: data,
		},
	}

	s.peerLock.Lock()
	peer, ok := s.peers[from]
	s.peerLock.Unlock()

	if !ok {
		return fmt.Errorf("peer not found: %s", from)
	}

	buf := new(bytes.Buffer)
	if err := gob.NewEncoder(buf).Encode(resp); err != nil {
		return err
	}

	return sendControlMessage(peer, buf.Bytes())
}

func (s *FileServer) performSelfHealing() {
	log.Printf("[%s] Starting deep fabric audit...", s.ID)

	files, err := s.Meta.GetAll()
	if err != nil {
		log.Printf("[%s] audit failed: %v", s.ID, err)
		return
	}

	activePeers := s.Peers()
	alive := make(map[string]bool)
	alive[s.ID] = true
	for _, p := range activePeers {
		alive[p] = true
	}

	for _, f := range files {
		missingIndices := []int{}
		for _, sm := range f.Shards {
			if !alive[sm.NodeID] {
				missingIndices = append(missingIndices, sm.Index)
			}
		}

		if len(missingIndices) == 0 {
			continue
		}

		log.Printf("[%s] ALERT: File %s is DEGRADED. Missing %d shards.", s.ID, f.Key, len(missingIndices))

		if len(missingIndices) > 2 {
			log.Printf("[%s] CRITICAL: File %s is unrecoverable (lost %d shards).", s.ID, f.Key, len(missingIndices))
			continue
		}

		// RECONSTRUCTION
		enc, _ := reedsolomon.New(3, 2)
		shards := make([][]byte, 5)
		have := 0

		for _, sm := range f.Shards {
			if alive[sm.NodeID] {
				data, err := s.GetShard(sm.NodeID, sm.CID)
				if err == nil {
					shards[sm.Index] = data
					have++
				}
			}
		}

		if have < 3 {
			log.Printf("[%s] FAILED: Not enough shards to heal %s.", s.ID, f.Key)
			continue
		}

		log.Printf("[%s] HEALING: Reconstructing %s...", s.ID, f.Key)
		if err := enc.Reconstruct(shards); err != nil {
			log.Printf("[%s] ERROR: Reconstruction failed for %s: %v", s.ID, f.Key, err)
			continue
		}

		// Re-distribute missing shards
		newShardManifest := append([]ShardMeta{}, f.Shards...)
		for _, idx := range missingIndices {
			targetNode := s.ID // Keep healed shards locally for efficiency
			newCID := hashKey(string(shards[idx]))

			// Encrypt before storing
			encrypted, _ := EncryptData(s.EncKey, shards[idx])
			if err := s.StoreShard(targetNode, newCID, bytes.NewReader(encrypted)); err == nil {
				newShardManifest[idx] = ShardMeta{
					Index:  idx,
					CID:    newCID,
					NodeID: targetNode,
				}
			}
		}

		// Sync healed manifest to mesh
		s.Meta.UpsertFile(f.Key, f.CID, f.OriginalSize, newShardManifest)
		s.BroadcastMetadata(FileRecord{
			Key:          f.Key,
			CID:          f.CID,
			OriginalSize: f.OriginalSize,
			Shards:       newShardManifest,
		})

		log.Printf("[%s] SELF-HEAL COMPLETE: %s is now back to 100%% redundancy.", s.ID, f.Key)
	}
}

type Message struct {
	Payload any
}

type MessageStoreFile struct {
	Key  string
	Data []byte
}

type MessageGetFile struct {
	Key string
}

type MessageGetFileResponse struct {
	Key  string
	Size int64
	Data []byte
}

type MessageMetadataSync FileRecord

func init() {
	gob.Register(MessageStoreFile{})
	gob.Register(MessageGetFile{})
	gob.Register(MessageGetFileResponse{})
	gob.Register(MessageMetadataSync{})
}

func sendControlMessage(peer p2p.Peer, payload []byte) error {
	if err := peer.Send([]byte{p2p.IncomingMessage}); err != nil {
		return err
	}

	if err := binary.Write(peer, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}

	return peer.Send(payload)
}
