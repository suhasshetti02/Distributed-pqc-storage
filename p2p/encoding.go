package p2p

import (
	"encoding/binary"
	"encoding/gob"
	"io"
)

type Decoder interface {
	Decode(io.Reader, *RPC) error
}

type GOBDecoder struct{}

func (dec GOBDecoder) Decode(r io.Reader, msg *RPC) error {
	return gob.NewDecoder(r).Decode(msg)
}

type DefaultDecoder struct{}

func (dec DefaultDecoder) Decode(r io.Reader, msg *RPC) error {
	peekBuf := make([]byte, 1)
	if _, err := io.ReadFull(r, peekBuf); err != nil {
		return err
	}
	// In case of a stream we are not decoding what is being sent over the network.
	// We are just setting Stream true so we can handle that in our logic.

	stream := peekBuf[0] == IncomingStream

	if stream {
		msg.Stream = true
		return nil
	}

	if peekBuf[0] != IncomingMessage {
		return io.ErrUnexpectedEOF
	}

	var payloadLen uint32
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		return err
	}
	buf := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	msg.Payload = buf

	return nil

}
