package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata"

	"github.com/Simplarix/dropwatch/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.NewRootCommand()
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "dropwatch:", err)
		os.Exit(1)
	}
}
