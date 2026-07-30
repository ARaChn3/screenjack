package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Server struct {
	srv      *http.Server
	port     int
	running  bool
	payloadPath string
	assetPath   string
}

func NewServer() *Server {
	return &Server{port: 8000}
}

func (s *Server) Start(payloadPath, assetPath string) error {
	if s.running {
		return nil
	}

	s.payloadPath = payloadPath
	s.assetPath = assetPath

	mux := http.NewServeMux()

	// Serve payload
	mux.HandleFunc("/"+filepath.Base(payloadPath), func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, payloadPath)
	})

	// Serve asset if set
	if assetPath != "" {
		mux.HandleFunc("/"+filepath.Base(assetPath), func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, assetPath)
		})
	}

	// Index page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
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
func PayloadExists(os string) (string, bool) {
	var path string
	if os == "windows" {
		path = "../dist/screenjack.exe"
	} else {
		path = "../dist/screenjack-linux"
	}
	_, err := os_stat(path)
	return path, err == nil
}

var os_stat = os.Stat
