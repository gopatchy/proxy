package proxy

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type Proxy struct {
	t        *testing.T
	backend  *net.TCPAddr
	listener *net.TCPListener

	conns  map[*net.TCPConn]bool
	refuse bool
	mu     sync.Mutex
}

func NewProxy(t *testing.T, backend *net.TCPAddr) *Proxy {
	var err error

	p := &Proxy{
		t:       t,
		backend: backend,
		conns:   map[*net.TCPConn]bool{},
	}

	p.listener, err = net.ListenTCP("tcp", nil)
	require.NoError(t, err)

	go p.accept()

	t.Logf("* -> %s -> [proxy] -> * -> %s listening...", p.listener.Addr(), p.backend)

	return p
}

func (p *Proxy) Addr() *net.TCPAddr {
	return p.listener.Addr().(*net.TCPAddr)
}

func (p *Proxy) HTTP() string {
	return fmt.Sprintf("http://%s/", p.Addr())
}

func (p *Proxy) HTTPS() string {
	return fmt.Sprintf("https://%s/", p.Addr())
}

func (p *Proxy) CloseAllConns() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.t.Logf("* -> %s -> [proxy] -> * -> %s closing all connections", p.listener.Addr(), p.backend)

	for conn := range p.conns {
		conn.Close()
	}

	p.conns = map[*net.TCPConn]bool{}
}

func (p *Proxy) SetRefuse(refuse bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if refuse {
		p.t.Logf("* -> %s -> [proxy] -> * -> %s refusing new connections", p.listener.Addr(), p.backend)
	} else {
		p.t.Logf("* -> %s -> [proxy] -> * -> %s accepting new connections", p.listener.Addr(), p.backend)
	}

	p.refuse = true
}

func (p *Proxy) SetBackend(backend *net.TCPAddr) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.backend = backend

	p.t.Logf("* -> %s -> [proxy] -> * -> %s switched to new backend", p.listener.Addr(), p.backend)
}

func (p *Proxy) Close() {
	p.mu.Lock()
	p.t.Logf("* -> %s -> [proxy] -> * -> %s closing...", p.listener.Addr(), p.backend)
	p.mu.Unlock()

	p.listener.Close()
	p.CloseAllConns()

	p.mu.Lock()
	p.t.Logf("* -> %s -> [proxy] -> * -> %s closed", p.listener.Addr(), p.backend)
	p.mu.Unlock()
}

func (p *Proxy) accept() {
	for {
		frontConn, err := p.listener.AcceptTCP()
		if err != nil {
			return
		}

		p.mu.Lock()

		if p.refuse {
			p.t.Logf("%s -> %s -> [proxy] -> * -> %s refused", frontConn.RemoteAddr(), frontConn.LocalAddr(), p.backend)
			frontConn.Close()
		} else {
			go p.dial(frontConn)
		}

		p.mu.Unlock()
	}
}

func (p *Proxy) dial(frontConn *net.TCPConn) {
	p.t.Logf("%s -> %s -> [proxy] -> * -> %s dialing...", frontConn.RemoteAddr(), frontConn.LocalAddr(), p.backend)

	backConn, err := net.DialTCP(p.backend.Network(), nil, p.backend)
	if err != nil {
		p.t.Logf("%s -> %s -> [proxy] -> * -> %s dialing failed: %s", frontConn.RemoteAddr(), frontConn.LocalAddr(), p.backend, err)
		frontConn.Close()
		return
	}

	p.t.Logf("%s -> %s -> [proxy] -> %s -> %s connected", frontConn.RemoteAddr(), frontConn.LocalAddr(), backConn.LocalAddr(), backConn.RemoteAddr())

	p.addConns(frontConn, backConn)

	go p.copy(frontConn, backConn)
	go p.copy(backConn, frontConn)
}

func (p *Proxy) copy(src, dest *net.TCPConn) {
	numBytes, err := dest.ReadFrom(src)

	if err == nil || errors.Is(err, net.ErrClosed) {
		p.t.Logf("%s -> %s -> [proxy] -> %s -> %s closed after %d bytes", src.RemoteAddr(), src.LocalAddr(), dest.LocalAddr(), dest.RemoteAddr(), numBytes)
	} else {
		p.t.Logf("%s -> %s -> [proxy] -> %s -> %s closed after %d bytes: %s", src.RemoteAddr(), src.LocalAddr(), dest.LocalAddr(), dest.RemoteAddr(), numBytes, err)
	}

	dest.Close()
	p.delConns(src)
}

func (p *Proxy) addConns(conns ...*net.TCPConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range conns {
		p.conns[conn] = true
	}
}

func (p *Proxy) delConns(conns ...*net.TCPConn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range conns {
		delete(p.conns, conn)
	}
}
