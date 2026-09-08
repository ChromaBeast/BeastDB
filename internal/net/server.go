package net

import (
	"io"
	"net"
	"sync"
)

// HandlerFunc processes an incoming Frame and produces a response Frame.
type HandlerFunc func(req *Frame) *Frame

// Server manages TCP client connections and routes frames to a handler.
type Server struct {
	addr     string
	listener net.Listener
	handler  HandlerFunc
	quit     chan struct{}
	wg       sync.WaitGroup
}

// NewServer initializes a new TCP database server.
func NewServer(addr string, handler HandlerFunc) *Server {
	return &Server{
		addr:    addr,
		handler: handler,
		quit:    make(chan struct{}),
	}
}

// Start binds to the TCP port and accepts client connections asynchronously.
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = l

	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// Addr returns the resolved network address of the active listener.
func (s *Server) Addr() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Stop initiates a graceful shutdown, closing listener and active connections.
func (s *Server) Stop() error {
	close(s.quit)
	var err error
	if s.listener != nil {
		err = s.listener.Close()
	}
	s.wg.Wait()
	return err
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	for {
		req, err := DecodeFrame(conn)
		if err != nil {
			if errorsIsEOF(err) {
				return
			}
			return
		}

		resp := s.handler(req)
		if resp != nil {
			respBytes := EncodeFrame(resp.OpCode, resp.Payload)
			if _, err := conn.Write(respBytes); err != nil {
				return
			}
		}
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || err == io.ErrUnexpectedEOF
}
