package proxy

import "net"

type Proxy struct {
	backend  *net.TCPAddr
	listener *net.TCPListener
}

func NewProxy(backend *net.TCPAddr) (*Proxy, error) {
	var err error

	p := &Proxy{
		backend: backend,
	}

	p.listener, err = net.ListenTCP("tcp", nil)
	if err != nil {
		return nil, err
	}

	go p.accept()

	return p, nil
}

func (p *Proxy) Close() {
	p.listener.Close()
}

func (p *Proxy) accept() {
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}

		conn.Close()
	}
}
