package connect

import (
	"io"
	"net"
	"time"
)

// streamConnRW wraps an io.ReadWriteCloser to implement net.Conn.
// This is used for HTTP/2 CONNECT response bodies that support bidirectional I/O.
type streamConnRW struct {
	rwc  io.ReadWriteCloser
	addr net.Addr
}

// newStreamConnRW creates a net.Conn from an io.ReadWriteCloser.
func newStreamConnRW(rwc io.ReadWriteCloser, remoteAddr net.Addr) net.Conn {
	return &streamConnRW{
		rwc:  rwc,
		addr: remoteAddr,
	}
}

// Read implements net.Conn.
func (c *streamConnRW) Read(b []byte) (n int, err error) {
	return c.rwc.Read(b)
}

// Write implements net.Conn.
func (c *streamConnRW) Write(b []byte) (n int, err error) {
	return c.rwc.Write(b)
}

// Close implements net.Conn.
func (c *streamConnRW) Close() error {
	return c.rwc.Close()
}

// LocalAddr implements net.Conn.
func (c *streamConnRW) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// RemoteAddr implements net.Conn.
func (c *streamConnRW) RemoteAddr() net.Addr {
	return c.addr
}

// SetDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConnRW) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConnRW) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConnRW) SetWriteDeadline(t time.Time) error {
	return nil
}

var _ net.Addr = (*remoteAddr)(nil)

type remoteAddr struct {
	addr string
}

func (a *remoteAddr) Network() string {
	return "connecttunnel"
}

func (a *remoteAddr) String() string {
	return a.addr
}
