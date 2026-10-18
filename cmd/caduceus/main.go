package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Uhm-why/caduceus/internal/config"
	"github.com/Uhm-why/caduceus/internal/mailbox"
	"github.com/Uhm-why/caduceus/internal/peer"
)

func main() {
	cfg, err := config.ParseAndLoad(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		log.Fatalf("Invalid configuration: %v", err)
	}

	mb, err := mailbox.New(cfg.MailboxPath)
	if err != nil {
		log.Fatalf("Failed to initialize mailbox at %s: %v", cfg.MailboxPath, err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	session := peer.NewSession(cfg, mb)
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
