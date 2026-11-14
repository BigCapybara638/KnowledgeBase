package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/BigCapybara638/KnowledgeBase/internal/mcpbridge"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := mcpbridge.Run(ctx, os.Getenv("KB_MCP_URL"), os.Getenv("KB_MCP_TOKEN"), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
