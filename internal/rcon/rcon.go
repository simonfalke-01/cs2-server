// Package rcon implements a minimal Source RCON client (the protocol used by
// CS2 dedicated servers) sufficient for authenticating and running commands
// like "status".
//
// Protocol reference: https://developer.valvesoftware.com/wiki/Source_RCON_Protocol
package rcon

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"
)

// Packet types per the Source RCON protocol.
const (
	typeExecCommand  = 2
	typeAuthResponse = 2
	typeAuth         = 3
)

const (
	maxPacketSize = 4096
	headerSize    = 10 // size of (id + type + 2 null terminators) once length is read
	// drainWindow bounds how long Exec waits for additional response packets
	// after the first one arrives. Source RCON splits responses larger than
	// ~4KB across multiple RESPONSE_VALUE packets with no built-in count, so we
	// keep reading until no further packet arrives within this short window.
	drainWindow = 250 * time.Millisecond
)

// ErrAuthFailed indicates the RCON password was rejected.
var ErrAuthFailed = errors.New("rcon: authentication failed")

// Client is a single-connection RCON client. It is not safe for concurrent use;
// create one per request or guard externally.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	nextID int32
}

// Dial connects to addr (host:port) and authenticates with password.
func Dial(ctx context.Context, addr, password string, timeout time.Duration) (*Client, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcon: dial: %w", err)
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	c := &Client{conn: conn, reader: bufio.NewReader(conn)}
	if err := c.authenticate(password); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) authenticate(password string) error {
	id := c.id()
	if err := c.write(id, typeAuth, password); err != nil {
		return err
	}

	// The server may send an empty RESPONSE_VALUE before the AUTH_RESPONSE.
	for {
		respID, respType, _, err := c.read()
		if err != nil {
			return fmt.Errorf("rcon: auth read: %w", err)
		}
		if respType == typeAuthResponse {
			if respID == -1 {
				return ErrAuthFailed
			}
			return nil
		}
		// Otherwise it was the throwaway RESPONSE_VALUE; keep reading.
	}
}

// Exec runs a console command and returns the server's text response.
//
// Source RCON has no per-response packet count: a large reply (e.g. "status"
// on a populated server, which can exceed 4KB) is split across several
// RESPONSE_VALUE packets. Reading a single packet would truncate that output
// and, downstream, feed the idle reaper a false-empty player count. We instead
// block for the first response packet, then keep draining additional packets
// until none arrives within a short window.
func (c *Client) Exec(cmd string) (string, error) {
	id := c.id()
	if err := c.write(id, typeExecCommand, cmd); err != nil {
		return "", err
	}

	// First packet: honour the connection-wide deadline set at Dial time.
	_, _, body, err := c.read()
	if err != nil {
		return "", fmt.Errorf("rcon: exec read: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(body)

	// Drain any continuation packets. Each iteration waits at most drainWindow
	// for more data; a read timeout means the response is complete.
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(drainWindow))
		_, _, more, derr := c.read()
		if derr != nil {
			var nerr net.Error
			if errors.As(derr, &nerr) && nerr.Timeout() {
				break // no further packets within the window: response complete
			}
			if errors.Is(derr, io.EOF) {
				break
			}
			return "", fmt.Errorf("rcon: exec drain: %w", derr)
		}
		sb.WriteString(more)
	}
	return sb.String(), nil
}

func (c *Client) id() int32 {
	return atomic.AddInt32(&c.nextID, 1)
}

// write encodes and sends a single RCON packet.
func (c *Client) write(id, packetType int32, body string) error {
	payload := make([]byte, 0, len(body)+headerSize)
	var idBuf [4]byte
	binary.LittleEndian.PutUint32(idBuf[:], uint32(id))
	payload = append(payload, idBuf[:]...)
	var typeBuf [4]byte
	binary.LittleEndian.PutUint32(typeBuf[:], uint32(packetType))
	payload = append(payload, typeBuf[:]...)
	payload = append(payload, []byte(body)...)
	payload = append(payload, 0x00, 0x00) // two null terminators

	var lenBuf [4]byte
	binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(payload)))

	if _, err := c.conn.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("rcon: write len: %w", err)
	}
	if _, err := c.conn.Write(payload); err != nil {
		return fmt.Errorf("rcon: write body: %w", err)
	}
	return nil
}

// read decodes a single RCON packet, returning its id, type and body.
func (c *Client) read() (id, packetType int32, body string, err error) {
	var lenBuf [4]byte
	if _, err = io.ReadFull(c.reader, lenBuf[:]); err != nil {
		return 0, 0, "", err
	}
	length := binary.LittleEndian.Uint32(lenBuf[:])
	if length < 8 || length > maxPacketSize {
		return 0, 0, "", fmt.Errorf("rcon: invalid packet length %d", length)
	}

	buf := make([]byte, length)
	if _, err = io.ReadFull(c.reader, buf); err != nil {
		return 0, 0, "", err
	}

	id = int32(binary.LittleEndian.Uint32(buf[0:4]))
	packetType = int32(binary.LittleEndian.Uint32(buf[4:8]))
	// Body is everything between offset 8 and the two trailing null bytes.
	if len(buf) >= 10 {
		body = string(buf[8 : len(buf)-2])
	}
	return id, packetType, body, nil
}

// Run is a convenience helper: dial, authenticate, run one command, close.
func Run(ctx context.Context, addr, password, cmd string, timeout time.Duration) (string, error) {
	c, err := Dial(ctx, addr, password, timeout)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	return c.Exec(cmd)
}
