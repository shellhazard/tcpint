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
	from          string
	to            string
	done          chan struct{}
	log           *log.Entry
	clienthandler func([]byte) []byte
	remotehandler func([]byte) []byte
	delimeter     byte

	// dynamic fields
	clientinjector []byte
	remoteinjector []byte

	sync.Mutex
}

func NewProxy(from, to string, clienthandler, remotehandler func([]byte) []byte, delimeter byte) *Proxy {
	return &Proxy{
		from: from,
		to:   to,
		done: make(chan struct{}),
		log: log.WithFields(log.Fields{
			"from": from,
			"to":   to,
		}),
		clienthandler:  clienthandler,
		remotehandler:  remotehandler,
		clientinjector: []byte{},
		remoteinjector: []byte{},
		delimeter:      delimeter,
	}
}

// Readers
func (p *Proxy) Stopped() bool {
	if p.done != nil {
		return false
	}
	return true
}

// Writers
func (p *Proxy) ToInject(b []byte) {
	p.Lock()
	defer p.Unlock()

	r := append(p.remoteinjector, b...)
	p.remoteinjector = r
}

func (p *Proxy) FromInject(b []byte) {
	p.Lock()
	defer p.Unlock()

	r := append(p.clientinjector, b...)
	p.clientinjector = r
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
	go p.intercept(connection, remote, "client", wg)
	// Pushing data to client from remote host
	go p.intercept(remote, connection, "remote", wg)
	wg.Wait()
}

// fn func([]byte) []byte, injector []byte
func (p *Proxy) intercept(from, to net.Conn, readertype string, wg *sync.WaitGroup) {
	defer wg.Done()
	// Create reader
	r := bufio.NewReader(from)

	// Set parameters
	var (
		fn       func([]byte) []byte
		injector []byte
	)
	switch readertype {
	case "client":
		fn = p.clienthandler
		injector = p.clientinjector
	case "remote":
		fn = p.remotehandler
		injector = p.remoteinjector
	}

	for {
		select {
		// If our proxy is stopped, return
		case <-p.done:
			break
		default:
			var buf []byte
			var err error

			// Read injected bytes
			if len(injector) > 0 {
				p.Lock()
				_ = copy(buf, injector)
				injector = nil
				p.Unlock()
				// Write injected bytes
				_, err = to.Write(buf)
				if err != nil {
					p.log.WithField("err", err).Errorln("Error writing injected bytes")
					p.Stop()
					break
				}
				buf = nil
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
