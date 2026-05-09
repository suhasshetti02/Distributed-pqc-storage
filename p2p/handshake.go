package p2p

import (
	"fmt"
	"io"

	"github.com/cloudflare/circl/kem/kyber/kyber768"
)

// HandshakeMessage represents the post-quantum handshake data.
type HandshakeMessage struct {
	NodeID    string
	PublicKey []byte // ML-KEM Public Key (for the initiator)
}

// HandshakeResponse represents the responder's side of the PQ exchange.
type HandshakeResponse struct {
	NodeID     string
	Ciphertext []byte // ML-KEM Ciphertext (containing the encapsulated secret)
}

// PostQuantumHandshake performs a PQ-secure key exchange using ML-KEM-768.
// It returns the remote NodeID and a shared secret key.
func PostQuantumHandshake(conn io.ReadWriter, localID string) (string, []byte, error) {
	// 1. Initiator generates ML-KEM key pair
	pk, sk, err := kyber768.Scheme().GenerateKeyPair()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate ML-KEM key: %v", err)
	}

	// 2. Send local NodeID and Public Key
	// We use a simple length-prefixed encoding for the handshake
	// NodeID (string), PK (fixed size for ML-KEM-768)
	// For simplicity in this demo, we'll use a fixed format
	
	// Send: [ID_LEN (1 byte)][ID (N bytes)][PK (1184 bytes)]
	idBytes := []byte(localID)
	if _, err := conn.Write([]byte{byte(len(idBytes))}); err != nil {
		return "", nil, err
	}
	if _, err := conn.Write(idBytes); err != nil {
		return "", nil, err
	}
	pkBytes, _ := pk.MarshalBinary()
	if _, err := conn.Write(pkBytes); err != nil {
		return "", nil, err
	}

	// 3. Receive Remote NodeID and Ciphertext
	// Recv: [ID_LEN (1 byte)][ID (N bytes)][CT (1088 bytes)]
	header := make([]byte, 1)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	remoteIDBytes := make([]byte, int(header[0]))
	if _, err := io.ReadFull(conn, remoteIDBytes); err != nil {
		return "", nil, err
	}
	
	ctBytes := make([]byte, kyber768.Scheme().CiphertextSize())
	if _, err := io.ReadFull(conn, ctBytes); err != nil {
		return "", nil, err
	}

	// 4. Decapsulate the secret
	sharedSecret, err := kyber768.Scheme().Decapsulate(sk, ctBytes)
	if err != nil {
		return "", nil, err
	}
	
	return string(remoteIDBytes), sharedSecret, nil
}

// PostQuantumHandshakeResponder handles the responder side of the PQ exchange.
func PostQuantumHandshakeResponder(conn io.ReadWriter, localID string) (string, []byte, error) {
	// 1. Receive Remote NodeID and Public Key
	header := make([]byte, 1)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", nil, err
	}
	remoteIDBytes := make([]byte, int(header[0]))
	if _, err := io.ReadFull(conn, remoteIDBytes); err != nil {
		return "", nil, err
	}
	
	pkBytes := make([]byte, kyber768.Scheme().PublicKeySize())
	if _, err := io.ReadFull(conn, pkBytes); err != nil {
		return "", nil, err
	}

	// 2. Encapsulate a secret for the received public key
	pk, err := kyber768.Scheme().UnmarshalBinaryPublicKey(pkBytes)
	if err != nil {
		return "", nil, err
	}
	
	ctBytes, sharedSecret, err := kyber768.Scheme().Encapsulate(pk)
	if err != nil {
		return "", nil, err
	}

	// 3. Send local NodeID and Ciphertext
	idBytes := []byte(localID)
	if _, err := conn.Write([]byte{byte(len(idBytes))}); err != nil {
		return "", nil, err
	}
	if _, err := conn.Write(idBytes); err != nil {
		return "", nil, err
	}
	if _, err := conn.Write(ctBytes); err != nil {
		return "", nil, err
	}

	return string(remoteIDBytes), sharedSecret, nil
}

