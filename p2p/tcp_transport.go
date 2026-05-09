package p2p

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
)

// TCPPeer represents the remote node over a TCP established connection.
type TCPPeer struct {
	net.Conn
	outbound bool
	remoteNodeID string
	mu sync.Mutex
}

func NewTCPPeer(conn net.Conn, outbound bool) *TCPPeer {
	return &TCPPeer{
		Conn:     conn,
		outbound: outbound,
	}
}

func (p *TCPPeer) RemoteNodeID() string {
	return p.remoteNodeID
}

func (p *TCPPeer) SetRemoteNodeID(id string) {
	p.remoteNodeID = id
}

func (p *TCPPeer) CloseStream() {}
func (p *TCPPeer) PauseDecoder(pause bool) {}

func (p *TCPPeer) Send(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.Conn.Write(b)
	return err
}

type TCPTransportOpts struct {
	NodeID     string
	ListenAddr string
	Decoder    Decoder
	OnPeer     func(Peer) error
}

type TCPTransport struct {
	TCPTransportOpts
	listener net.Listener
	rpcch    chan RPC
}

func NewTCPTransport(opts TCPTransportOpts) *TCPTransport {
	return &TCPTransport{
		TCPTransportOpts: opts,
		rpcch:            make(chan RPC, 1024),
	}
}

func (t *TCPTransport) Addr() string {
	return t.ListenAddr
}

func (t *TCPTransport) Consume() <-chan RPC {
	return t.rpcch
}

func (t *TCPTransport) Close() error {
	return t.listener.Close()
}

func (t *TCPTransport) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	go t.handleConn(conn, true)
	return nil
}

func (t *TCPTransport) ListenAndAccept() error {
	var err error
	t.listener, err = net.Listen("tcp", t.ListenAddr)
	if err != nil {
		return err
	}
	go t.startAcceptLoop()
	log.Printf("TCP transport listening on port: %s\n", t.ListenAddr)
	return nil
}

func (t *TCPTransport) startAcceptLoop() {
	for {
		conn, err := t.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			fmt.Printf("TCP accept error: %s\n", err)
		}
		go t.handleConn(conn, false)
	}
}

func (t *TCPTransport) handleConn(conn net.Conn, outbound bool) {
	var err error
	defer conn.Close()

	peer := NewTCPPeer(conn, outbound)
	var (
		remoteID     string
		sharedSecret []byte
	)

	if outbound {
		remoteID, sharedSecret, err = PostQuantumHandshake(conn, t.NodeID)
	} else {
		remoteID, sharedSecret, err = PostQuantumHandshakeResponder(conn, t.NodeID)
	}

	if err != nil {
		return
	}
	
	log.Printf("[%s] PQ handshake success with %s. Secret derived.", t.NodeID, remoteID)
	_ = sharedSecret // acknowledge for compiler
	peer.SetRemoteNodeID(remoteID)
	if t.OnPeer != nil {
		if err = t.OnPeer(peer); err != nil {
			return
		}
	}

	for {
		rpc := RPC{}
		err = t.Decoder.Decode(conn, &rpc)
		if err != nil {
			return
		}
		rpc.From = peer.RemoteNodeID()
		t.rpcch <- rpc
	}
}
