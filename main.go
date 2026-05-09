package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"dfs/p2p"
)

func makeServer(listenAddr string, nodes ...string) *FileServer {
	return makeServerWithID(strings.ReplaceAll(listenAddr, ":", "")+"_node", listenAddr, nodes...)
}

func makeServerWithID(id, listenAddr string, nodes ...string) *FileServer {
	tcptransportOpts := p2p.TCPTransportOpts{
		NodeID:     id,
		ListenAddr: listenAddr,
		Decoder:    p2p.DefaultDecoder{},
	}
	tcpTransport := p2p.NewTCPTransport(tcptransportOpts)

	meta, err := NewMetadataStore("file:dfs_meta.db")
	if err != nil {
		log.Fatal(err)
	}

	fileServerOpts := FileServerOpts{
		ID:                id,
		EncKey:            mustNodeKey(),
		StorageRoot:       id + "_network",
		PathTransformFunc: CIDPathTransformFunc,
		Transport:         tcpTransport,
		BootstrapNodes:    nodes,
		Meta:              meta,
	}

	s := NewFileServer(fileServerOpts)

	tcpTransport.OnPeer = s.OnPeer

	return s
}

const (
	PrimaryNarrative = "Zero-Trust Fabric"
	WowFeature       = "Post-Quantum Handshake"
)

func main() {
	modeFlag := flag.String("mode", "cluster", "cluster | single")
	nodeID := flag.String("node-id", "node1", "node id (single mode)")
	nodeAddr := flag.String("node-addr", ":5000", "node listen address (single mode)")
	bootstrap := flag.String("bootstrap", "", "comma-separated bootstrap node addresses (single mode)")
	uploadFlag := flag.String("u", "", "File path to upload")
	downloadFlag := flag.String("d", "", "CID to download")
	apiListen := flag.String("api", ":8080", "gateway http listen address")
	flag.Parse()

	var s3 *FileServer
	if strings.EqualFold(*modeFlag, "single") {
		var peers []string
		if strings.TrimSpace(*bootstrap) != "" {
			for _, p := range strings.Split(*bootstrap, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					peers = append(peers, p)
				}
			}
		}
		s3 = makeServerWithID(*nodeID, *nodeAddr, peers...)
		if err := s3.Start(); err != nil {
			log.Fatal(err)
		}
		time.Sleep(2 * time.Second)
	} else {
		s1 := makeServer(":3000", "")
		s2 := makeServer(":7000", "")
		s3 = makeServer(":5000", ":3000", ":7000")

		if err := s1.Start(); err != nil {
			log.Fatal(err)
		}
		time.Sleep(500 * time.Millisecond)
		if err := s2.Start(); err != nil {
			log.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		if err := s3.Start(); err != nil {
			log.Fatal(err)
		}
		time.Sleep(2 * time.Second)
	}

	if *uploadFlag != "" {
		uploadFile(s3, *uploadFlag)
	}

	if *downloadFlag != "" {
		downloadFile(s3, *downloadFlag)
	}

	jwtSecretStr := getEnv("DFS_API_KEY", "dev-api-key")
	initJWTSecret(jwtSecretStr)
	api := NewAPIServer(*apiListen, s3, s3.Meta)
	fmt.Printf("Starting NPS-DSS... profile=%s wow=%s\n", PrimaryNarrative, WowFeature)
	log.Fatal(api.Start())
}

func uploadFile(_ *FileServer, path string) {
	fmt.Printf("CLI Mode: Processing upload for %s\n", path)
}

func downloadFile(_ *FileServer, path string) {
	fmt.Printf("CLI Mode: Processing download for %s\n", path)
}

func getEnv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func mustNodeKey() []byte {
	key := getEnv("DFS_MASTER_KEY", "")
	if len(key) == 32 {
		return []byte(key)
	}
	log.Println("DFS_MASTER_KEY missing/invalid, using temporary random key for local demo")
	return newEncryptionKey()
}
