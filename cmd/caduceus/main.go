package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Uhm-why/caduceus/internal/peer"
)

func main() {
	var ipFlag string
	var configFlag string
	configDir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		configDir = filepath.Join(home, ".config")
	}

	defaultConfigDir := filepath.Join(configDir, "caduceus")
	var listenFlag string
	var sanFlag string
	flag.StringVar(&listenFlag, "listen", "", "Address to bind for incoming peer connections (e.g. :8443)")
	flag.StringVar(&ipFlag, "ip", "", "The ip and port [0.0.0.0:0000] of the other machine running hermes to connect to.")
	flag.StringVar(&configFlag, "config", defaultConfigDir, "The directory where your caduceus certificates are stored.")
	flag.StringVar(&sanFlag, "san", "caduceus-agent", "Expected Subject Alternative Name (SAN)")

	flag.Parse()

	if listenFlag == "" && ipFlag == "" {
		log.Fatalf("Caduceus requires at least -listen or -ip (run with -help for usage)")
	}

	if ipFlag != "" {
		if _, _, err := net.SplitHostPort(ipFlag); err != nil {
			log.Fatalf("Invalid -ip target %q (expected host:port): %v", ipFlag, err)
		}
	}

	cfg := &peer.Config{
		ListenAddress:    listenFlag,
		PeerAddress:      ipFlag,
		TLSCertPath:      filepath.Join(configFlag, "cert.pem"),
		TLSKeyPath:       filepath.Join(configFlag, "key.pem"),
		CAPath:           filepath.Join(configFlag, "ca.pem"),
		PeerSAN:          sanFlag,
		HandshakeTimeout: 5 * time.Second,
		DialTimeout:      5 * time.Second,
		IdleTimeout:      60 * time.Second,
		KeepAlive:        15 * time.Second,
		MaxFrameSize:     10 * 1024 * 1024,
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	session := peer.NewSession(cfg)
	dialer := peer.NewDialer(cfg)
	bridge := peer.NewBridge(session, dialer)

	if cfg.ListenAddress != "" {
		listener := peer.NewListener(cfg)
		connChan, errChan, err := listener.Start(ctx)
		if err != nil {
			log.Fatalf("Failed to start listener on %s: %v", cfg.ListenAddress, err)
		}

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case err := <-errChan:
					log.Printf("Listener error: %v", err)
				case conn, ok := <-connChan:
					if !ok {
						return
					}
					if err := session.Attach(ctx, conn); err != nil {
						log.Printf("Failed to attach inbound connection: %v", err)
					} else {
						log.Printf("Accepted peer connection from %s", conn.RemoteAddr())
					}
				}
			}
		}()
	}

	if err := bridge.Serve(ctx); err != nil {
		log.Printf("MCP Bridge error: %v", err)
	}
	cancel()
	_ = session.Close()
}
