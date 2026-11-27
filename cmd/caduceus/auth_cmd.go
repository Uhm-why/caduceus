package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/Uhm-why/caduceus/internal/auth"
	"github.com/Uhm-why/caduceus/internal/config"
	"github.com/Uhm-why/caduceus/internal/mailbox"
)

var (
	defaultAuthDir     = config.DefaultConfigDir()
	defaultPolicyPath  = filepath.Join(defaultAuthDir, "policy.json")
	defaultMailboxPath = filepath.Join(defaultAuthDir, "mailbox")
)

// runAuthRules dispatches the "auth rules" subcommand. It loads the policy configuration
// and displays the default action and all configured rules in priority order.
func runAuthRules(args []string) {
	fs := flag.NewFlagSet("auth rules", flag.ExitOnError)
	policyPath := fs.String("policy", defaultPolicyPath, "Path to policy.json")
	_ = fs.Parse(args)

	p, err := auth.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading policy.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Default Action: %s\n", p.DefaultAction.String())

	rules := p.RulesSnapshot()

	if len(rules) == 0 {
		fmt.Printf("No custom policy rules configured.\n")
		return
	}

	writer := tabwriter.NewWriter(os.Stdout, 5, 4, 2, '	', tabwriter.DiscardEmptyColumns)

	headerText := "PRIORITY	PATTERN	ACTION	DESCRIPTION\n"

	_, _ = fmt.Fprint(writer, headerText)

	priority := 1
	for _, rule := range rules {
		fmt.Fprintf(writer, "%d	%s	%s	%s\n", priority, rule.Pattern, rule.Action, rule.Description)
		priority++
	}

	_ = writer.Flush()
}

// runAuthAllow dispatches the "auth allow" subcommand. It loads the policy configuration,
// validates the pattern, prepends an allow rule, and saves the updated policy to disk.
func runAuthAllow(args []string) {
	fs := flag.NewFlagSet("auth allow", flag.ExitOnError)
	policyPath := fs.String("policy", defaultPolicyPath, "Path to policy.json")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Pattern is a required argument.\n")
		os.Exit(1)
	}

	pattern := fs.Arg(0)

	description := ""
	if fs.NArg() > 1 {
		description = fs.Arg(1)
	}

	p, err := auth.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading policy.json: %v\n", err)
		os.Exit(1)
	}

	err = p.AddRule(auth.PolicyRule{
		Pattern:     pattern,
		Action:      auth.PolicyAllow,
		Description: description,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while adding rule: %v\n", err)
		os.Exit(1)
	}

	err = auth.SavePolicy(*policyPath, p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while saving policy: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Policy rule added: %s -> %s\n", pattern, auth.PolicyAllow.String())

}

// runAuthDeny dispatches the "auth deny" subcommand. It loads the policy configuration,
// validates the pattern, prepends a deny rule, and saves the updated policy to disk.
func runAuthDeny(args []string) {
	fs := flag.NewFlagSet("auth deny", flag.ExitOnError)
	policyPath := fs.String("policy", defaultPolicyPath, "Path to policy.json")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Pattern is a required argument.\n")
		os.Exit(1)
	}

	pattern := fs.Arg(0)

	description := ""
	if fs.NArg() > 1 {
		description = fs.Arg(1)
	}

	p, err := auth.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading policy.json: %v\n", err)
		os.Exit(1)
	}

	err = p.AddRule(auth.PolicyRule{
		Pattern:     pattern,
		Action:      auth.PolicyDeny,
		Description: description,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while adding rule: %v\n", err)
		os.Exit(1)
	}

	err = auth.SavePolicy(*policyPath, p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while saving policy: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Policy rule added: %s -> %s\n", pattern, auth.PolicyDeny.String())
}

// runAuthRemove dispatches the "auth remove" subcommand. It removes a rule matching the
// given pattern from the policy and saves the updated policy to disk.
func runAuthRemove(args []string) {
	fs := flag.NewFlagSet("auth remove", flag.ExitOnError)
	policyPath := fs.String("policy", defaultPolicyPath, "Path to policy.json")
	_ = fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Pattern is a required argument.\n")
		os.Exit(1)
	}

	pattern := fs.Arg(0)

	p, err := auth.LoadPolicy(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while reading policy.json: %v\n", err)
		os.Exit(1)
	}

	removed := p.RemoveRule(pattern)

	if !removed {
		fmt.Printf("Rule pattern not found.\n")
		os.Exit(0)
	}

	err = auth.SavePolicy(*policyPath, p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error while saving policy: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Policy rule removed: %s\n", pattern)

}

// runAuthPending dispatches the "auth pending" subcommand. It scans the Maildir new/
// spool for unhandled authorization challenge messages and displays them.
func runAuthPending(args []string) {
	fs := flag.NewFlagSet("auth pending", flag.ExitOnError)
	mailboxPath := fs.String("mailbox", defaultMailboxPath, "Path to mailbox directory.")
	_ = fs.Parse(args)

	readPath := filepath.Join(*mailboxPath, "new")

	dirContents, err := os.ReadDir(readPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("No pending authorization challenges.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error reading maildir contents: %v\n", err)
		os.Exit(1)
	}

	authCount := 0

	for _, file := range dirContents {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(readPath, file.Name())

		if filepath.Ext(filePath) != ".json" {
			continue
		}

		fileBytes, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %s\n", file.Name())
			continue
		}

		message := &mailbox.Message{}

		_ = json.Unmarshal(fileBytes, &message)

		if message.Kind == "auth_challenge" {
			fmt.Printf("Task ID: %s, Timestamp: %s, Details: %v\n", message.CorrelationID, message.Timestamp.String(), message.Content)
			authCount++
		}
	}

	if authCount == 0 {
		fmt.Printf("No pending authorization challenges.\n")
	}

}

// runAuth is the CLI router for authorization operations. It inspects the first positional
// argument and dispatches to the corresponding auth subcommand handler.
func runAuth(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: caduceus auth <rules|allow|deny|remove|pending> [options]")
		os.Exit(1)
	}

	switch args[0] {
	case "rules":
		runAuthRules(args[1:])
	case "allow":
		runAuthAllow(args[1:])
	case "deny":
		runAuthDeny(args[1:])
	case "remove":
		runAuthRemove(args[1:])
	case "pending":
		runAuthPending(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown auth subcommand: %s\n", args[0])
		os.Exit(1)
	}
}
