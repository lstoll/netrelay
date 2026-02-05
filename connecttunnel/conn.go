package connecttunnel

import (
	"fmt"
	"io"
	"net"
	"time"
)

// tunnelConn wraps a net.Conn to provide proper cleanup and coordination.
type tunnelConn struct {
	net.Conn
	closer io.Closer // Optional additional resource to close
}

// newTunnelConn creates a new tunnel connection wrapper.
func newTunnelConn(conn net.Conn, closer io.Closer) net.Conn {
	return &tunnelConn{
		Conn:   conn,
		closer: closer,
	}
}

// Close closes both the underlying connection and any additional resources.
func (c *tunnelConn) Close() error {
	err1 := c.Conn.Close()
	if c.closer != nil {
		err2 := c.closer.Close()
		if err1 == nil {
			return err2
		}
	}
	return err1
}

// h2ConnWrapper wraps an HTTP/2 response body to provide both reading and writing.
// For HTTP/2 CONNECT, the response body provides the read side of the tunnel,
// while writes must go through the original request's pipe.
type h2ConnWrapper struct {
	io.ReadCloser
	remoteAddr net.Addr
}

// streamConn wraps an io.ReadCloser to implement net.Conn.
// This is used for HTTP/2 CONNECT response bodies which only provide reading.
type streamConn struct {
	body io.ReadCloser
	addr net.Addr
}

// newStreamConn creates a net.Conn from an io.ReadCloser.
func newStreamConn(body io.ReadCloser, remoteAddr net.Addr) net.Conn {
	return &streamConn{
		body: body,
		addr: remoteAddr,
	}
}

// Read implements net.Conn.
func (c *streamConn) Read(b []byte) (n int, err error) {
	return c.body.Read(b)
}

// Write implements net.Conn.
// For HTTP/2 CONNECT, writes are not supported through the response body.
func (c *streamConn) Write(b []byte) (n int, err error) {
	return 0, fmt.Errorf("connecttunnel: write not supported on HTTP/2 response body")
}

// Close implements net.Conn.
func (c *streamConn) Close() error {
	return c.body.Close()
}

// LocalAddr implements net.Conn.
// Returns a dummy address as HTTP/2 streams don't have distinct local addresses.
func (c *streamConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

// RemoteAddr implements net.Conn.
func (c *streamConn) RemoteAddr() net.Addr {
	return c.addr
}

// SetDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline implements net.Conn.
// Not supported for HTTP/2 streams, returns nil.
func (c *streamConn) SetWriteDeadline(t time.Time) error {
	return nil
}

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
