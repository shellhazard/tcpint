package tcpint

import (
	"bufio"
	"net"
	"sync"

	log "github.com/sirupsen/logrus"
)

const (
	NULLBYTE byte = 0
)

type Proxy struct {
	from        string
	to          string
	done        chan struct{}
	log         *log.Entry
	fromhandler func([]byte) []byte
	tohandler   func([]byte) []byte
	delimeter   byte

	// dynamic fields

	sync.Mutex
}

func NewProxy(from, to string, fromhandler, tohandler func([]byte) []byte, delimeter byte) *Proxy {
	return &Proxy{
		from: from,
		to:   to,
		done: make(chan struct{}),
		log: log.WithFields(log.Fields{
			"from": from,
			"to":   to,
		}),
		fromhandler:  fromhandler,
		tohandler:    tohandler,
		frominjector: []byte{},
		toinjector:   []byte{},
		delimeter:    delimeter,
	}
}

func (p *Proxy) ToInject(b []byte) {
	p.Lock()
	defer p.Unlock()

	r := append(p.toinjector, b...)
	p.toinjector = r
}

func (p *Proxy) FromInject(b []byte) {
	p.Lock()
	defer p.Unlock()

	r := append(p.frominjector, b...)
	p.frominjector = r
}

// Start proxy server
func (p *Proxy) Start() error {
	p.log.Infoln("Starting proxy on", p.from)
	listener, err := net.Listen("tcp", p.from)
	if err != nil {
		return err
	}
	go p.run(listener)
	return nil
}

// Stop proxy server
func (p *Proxy) Stop() {
	// Close channel
	if p.done == nil {
		return
	}
	p.log.Infoln("Stopping proxy")
	close(p.done)
	p.done = nil
}

func (p *Proxy) run(listener net.Listener) {
	for {
		select {
		// If our proxy is stopped, return
		case <-p.done:
			return
		default:
			connection, err := listener.Accept()
			if err == nil {
				p.log.Infoln("New connection")
				go p.handle(connection)
			} else {
				p.log.WithField("err", err).Errorln("Error accepting conn")
			}
		}
	}
}

func (p *Proxy) handle(connection net.Conn) {
	// New incoming connection from a client
	p.log.Debugln("Handling", connection)
	defer p.log.Debugln("Done handling", connection)
	defer connection.Close()
	// Connect to remote server
	remote, err := net.Dial("tcp", p.to)
	if err != nil {
		p.log.WithField("err", err).Errorln("Error dialing remote host")
		return
	}
	defer remote.Close()
	// Create a new waitgroup
	wg := &sync.WaitGroup{}
	wg.Add(2)
	// Pushing data from client to remote host
	go p.intercept(connection, remote, p.fromhandler, *p.frominjector, wg)
	// Pushing data to client from remote host
	go p.intercept(remote, connection, p.tohandler, *p.toinjector, wg)
	wg.Wait()
}

func (p *Proxy) intercept(from, to net.Conn, fn func([]byte) []byte, injector *[]byte, wg *sync.WaitGroup) {
	defer wg.Done()
	// Create reader
	r := bufio.NewReader(from)
	select {
	// If our proxy is stopped, return
	case <-p.done:
		break
	default:
		for {
			var buf []byte

			// Read injected bytes
			if len(injector) > 0 {
				p.Lock()
				_ = copy(buf, injector)
				injector = nil
				p.Unlock()
			}

			// Read bytes up to delimeter
			buf, err = r.ReadBytes(p.delimeter)
			if err != nil {
				p.log.WithField("err", err).Errorln("Error from reader")
				p.Stop()
				break
			}

			// Run process function
			modbuf := fn(buf)

			// Write bytes to other side
			_, err = to.Write(modbuf)
			if err != nil {
				p.log.WithField("err", err).Errorln("Error writing")
				p.Stop()
				break
			}
		}
	}
}
