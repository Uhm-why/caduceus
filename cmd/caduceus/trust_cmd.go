package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Uhm-why/caduceus/internal/config"
	"github.com/Uhm-why/caduceus/internal/trust"
)

var (
	defaultCADir   = config.DefaultConfigDir()
	defaultCAPath  = filepath.Join(defaultCADir, "ca.pem")
	defaultKeyPath = filepath.Join(defaultCADir, "ca.key")
)

// runTrustList dispatches the "trust list" subcommand. It parses the -ca and -key
// flags, loads the certificates from the CA bundle, and prints their Subject CN,
// SHA-256 fingerprint, and local root status to stdout.
func runTrustList(args []string) {
	fs := flag.NewFlagSet("trust list", flag.ExitOnError)
	caPath := fs.String("ca", defaultCAPath, "Path to CA bundle")
	keyPath := fs.String("key", defaultKeyPath, "Path to local CA private key")
	_ = fs.Parse(args)

	trustedCerts, err := trust.LoadCertificates(*caPath, *keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while loading certificates: %v\n", err)
		os.Exit(1)
	}

	if len(trustedCerts) == 0 {
		fmt.Printf("No trusted certificates...\n")
		return
	}
	for _, cert := range trustedCerts {
		fmt.Printf("Subject CN: %s\nFingerprint: %s\nLocal Root: %t\n", cert.SubjectCN, cert.Fingerprint, cert.IsLocalRoot)
	}
}

// runTrustAdd dispatches the "trust add" subcommand. It parses the -ca flag,
// reads a PEM-encoded CA certificate from the specified file path, validates it,
// and atomically appends it to the CA bundle.
func runTrustAdd(args []string) {
	fs := flag.NewFlagSet("trust add", flag.ExitOnError)
	caPath := fs.String("ca", defaultCAPath, "Path to CA bundle")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Path to certificate file is required\n")
		os.Exit(1)
	}

	certData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading certificate file: %v\n", err)
		os.Exit(1)
	}

	added, err := trust.AddCertificate(*caPath, certData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while adding certificate to CA: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Certificate added:\n    Common Name: %s\n    Fingerprint: %s\n", added.SubjectCN, added.Fingerprint)
}

// runTrustRemove dispatches the "trust remove" subcommand. It parses the -ca and
// -key flags and atomically removes the certificate matching the specified SHA-256
// fingerprint from the CA bundle, refusing to remove the local node's root CA.
func runTrustRemove(args []string) {
	fs := flag.NewFlagSet("trust remove", flag.ExitOnError)
	caPath := fs.String("ca", defaultCAPath, "Path to CA bundle")
	keyPath := fs.String("key", defaultKeyPath, "Path to CA private key")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Fingerprint of certificate to be removed is required\n")
		os.Exit(1)
	}

	err := trust.RemoveCertificate(*caPath, *keyPath, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while removing certificate: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Certificate with fingerprint %s removed.\n", fs.Arg(0))
}

// runTrust is the CLI router for trust management operations. It inspects the
// first positional argument and dispatches to the corresponding list, add, or
// remove subcommand handler.
func runTrust(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: caduceus trust <list|add|remove> [options]")
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		runTrustList(args[1:])
	case "add":
		runTrustAdd(args[1:])
	case "remove":
		runTrustRemove(args[1:])
	default:
		fmt.Printf("Unknown command\n")
		os.Exit(1)
	}
}
