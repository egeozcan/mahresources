//go:build postgres

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"mahresources/internal/testpgutil"
)

func main() {
	ctx := context.Background()

	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(container.DSN())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintf(os.Stderr, "Shutting down postgres container...\n")
	if err := container.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping container: %v\n", err)
		os.Exit(1)
	}
}
