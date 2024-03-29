package chinadns

import (
	"fmt"
	"github.com/miekg/dns"
)

type Config struct {
	FastDNS  string
	CleanDNS string
	IP       string
	Port     int
}

type Server struct {
	config Config
	s      *dns.Server
}

// Error type for custom error handling.
type Error string

// Error method satisfies the error interface for Error type.
func (e Error) Error() string {
	return string(e)
}

func NewServer(config Config) (*Server, error) {
	s := &Server{config: config}
	if s.config.IP == "" {
		s.config.IP = "127.0.0.1"
	}
	if s.config.Port == 0 {
		s.config.Port = 53
	}
	s.s = &dns.Server{
		Addr: fmt.Sprintf("%s:%d", s.config.IP, s.config.Port),
		Net:  "udf",
	}
	dns.HandleFunc(".", s.handleDNSQuery)

	return s, nil
}

// Run starts the DNS server.
func (s *Server) Run() error {
	return s.s.ListenAndServe()
}

// handleDNSQuery processes DNS queries.
func (s *Server) handleDNSQuery(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	w.WriteMsg(m)
}
