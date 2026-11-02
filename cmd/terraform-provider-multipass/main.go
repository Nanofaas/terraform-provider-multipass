package main

import (
	"context"
	"log"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/Nanofaas/terraform-provider-multipass/internal/provider"
)

var (
	// version is set at build time through -ldflags and defaults to dev.
	version = "dev"
)

func main() {
	opts := providerserver.ServeOpts{
		Address: "registry.opentofu.org/nanofaas/multipass",
	}

	if err := providerserver.Serve(
		context.Background(),
		provider.New(version),
		opts,
	); err != nil {
		log.Printf("error serving provider: %v", err)
		os.Exit(1)
	}
}
