package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/Nanofaas/terraform-provider-multipass/internal/preflight"
	"github.com/Nanofaas/terraform-provider-multipass/internal/sshclient"
	"os"
	"time"
)

func main() {
	var cfg sshclient.Config
	flag.StringVar(&cfg.Host, "host", "", "Linux SSH host or configured alias (required)")
	flag.StringVar(&cfg.User, "user", "", "SSH user (otherwise use SSH configuration)")
	flag.IntVar(&cfg.Port, "port", 0, "SSH port (otherwise use SSH configuration)")
	flag.StringVar(&cfg.IdentityFile, "identity-file", "", "Local private-key path")
	flag.StringVar(&cfg.KnownHostsFile, "known-hosts-file", "", "Local known_hosts path")
	binary := flag.String("multipass-path", "multipass", "Remote Multipass CLI path")
	networks := flag.Bool("networks", false, "Also check managed-network helper, privileges and firewall")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Unexpected positional arguments")
		os.Exit(2)
	}
	ssh, err := sshclient.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	failed := false
	for _, check := range preflight.Run(ctx, ssh, *binary, *networks) {
		fmt.Printf("%-4s %-16s %s\n", check.Status, check.Name, check.Detail)
		failed = failed || check.Status == "FAIL"
	}
	if failed {
		os.Exit(1)
	}
}
