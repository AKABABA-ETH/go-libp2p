package protocol

import (
	"fmt"
	"io"
	"sync"

	context "github.com/jbenet/go-ipfs/Godeps/_workspace/src/code.google.com/p/go.net/context"

	inet "github.com/jbenet/go-ipfs/p2p/net2"
	eventlog "github.com/jbenet/go-ipfs/util/eventlog"
	lgbl "github.com/jbenet/go-ipfs/util/eventlog/loggables"
)

var log = eventlog.Logger("net/mux")

type StreamHandlerMap map[ID]inet.StreamHandler

// Mux provides simple stream multixplexing.
// It helps you precisely when:
//  * You have many streams
//  * You have function handlers
//
// It contains the handlers for each protocol accepted.
// It dispatches handlers for streams opened by remote peers.
//
// WARNING: this datastructure IS NOT threadsafe.
// do not modify it once the network is using it.
type Mux struct {
	Default  inet.StreamHandler // handles unknown protocols.
	Handlers StreamHandlerMap

	sync.RWMutex
}

// Protocols returns the list of protocols this muxer has handlers for
func (m *Mux) Protocols() []ID {
	m.RLock()
	l := make([]ID, 0, len(m.Handlers))
	for p := range m.Handlers {
		l = append(l, p)
	}
	m.RUnlock()
	return l
}

// readHeader reads the stream and returns the next Handler function
// according to the muxer encoding.
func (m *Mux) readHeader(s io.Reader) (ID, inet.StreamHandler, error) {
	// log.Error("ReadProtocolHeader")
	p, err := ReadHeader(s)
	if err != nil {
		return "", nil, err
	}

	// log.Debug("readHeader got:", p)
	m.RLock()
	h, found := m.Handlers[p]
	m.RUnlock()

	switch {
	case !found && m.Default != nil:
		return p, m.Default, nil
	case !found && m.Default == nil:
		return p, nil, fmt.Errorf("%s no handler with name: %s (%d)", m, p, len(p))
	default:
		return p, h, nil
	}
}

// String returns the muxer's printing representation
func (m *Mux) String() string {
	m.RLock()
	defer m.RUnlock()
	return fmt.Sprintf("<Muxer %p %d>", m, len(m.Handlers))
}

// SetHandler sets the protocol handler on the Network's Muxer.
// This operation is threadsafe.
func (m *Mux) SetHandler(p ID, h inet.StreamHandler) {
	log.Debugf("%s setting handler for protocol: %s (%d)", m, p, len(p))
	m.Lock()
	m.Handlers[p] = h
	m.Unlock()
}

// Handle reads the next name off the Stream, and calls a handler function
// This is done in its own goroutine, to avoid blocking the caller.
func (m *Mux) Handle(s inet.Stream) {
	go m.HandleSync(s)
}

// HandleSync reads the next name off the Stream, and calls a handler function
// This is done synchronously. The handler function will return before
// HandleSync returns.
func (m *Mux) HandleSync(s inet.Stream) {
	ctx := context.Background()

	name, handler, err := m.readHeader(s)
	if err != nil {
		err = fmt.Errorf("protocol mux error: %s", err)
		log.Error(err)
		log.Event(ctx, "muxError", lgbl.Error(err))
		s.Close()
		return
	}

	log.Infof("muxer handle protocol: %s", name)
	log.Event(ctx, "muxHandle", eventlog.Metadata{"protocol": name})
	handler(s)
}

// ReadLengthPrefix reads the name from Reader with a length-byte-prefix.
func ReadLengthPrefix(r io.Reader) (string, error) {
	// c-string identifier
	// the first byte is our length
	l := make([]byte, 1)
	if _, err := io.ReadFull(r, l); err != nil {
		return "", err
	}
	length := int(l[0])

	// the next are our identifier
	name := make([]byte, length)
	if _, err := io.ReadFull(r, name); err != nil {
		return "", err
	}

	return string(name), nil
}
