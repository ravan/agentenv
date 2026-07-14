package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ravan/agentenv/internal/cliapp"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "agentenv:", err)

		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) {
			os.Exit(exitCoder.ExitCode())
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	return cliapp.New(cliapp.Options{}).Run(ctx, args)
}
