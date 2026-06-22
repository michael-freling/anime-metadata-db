// Command builder compiles the authored override layer plus pinned open-data
// sources into the generated data/ dataset.
//
// Usage:
//
//	builder init                 download the pinned sources into the cache
//	builder build [id...]        (re)build all overrides, or just the given ids
//	builder refresh              update sources to latest + rebuild everything
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/michael-freling/anime-metadata-db/internal/builder"
)

func main() {
	os.Exit(run(os.Args[1:], nil, os.Stdout, os.Stderr))
}

// run builds and executes the root command. fetcher may be nil to use a real
// HTTP client. It returns the process exit code.
func run(args []string, fetcher builder.Fetcher, stdout, stderr io.Writer) int {
	root := newRootCmd(fetcher)
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

// newRootCmd assembles the cobra command tree. A nil fetcher makes each command
// use a real HTTP client.
func newRootCmd(fetcher builder.Fetcher) *cobra.Command {
	var dir string

	root := &cobra.Command{
		Use:           "builder",
		Short:         "Compile anime franchise overrides into the open dataset",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&dir, "dir", ".", "repo root directory")

	newApp := func(cmd *cobra.Command) *builder.App {
		return builder.New(dir, fetcher, cmd.OutOrStdout())
	}

	root.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: "Download the pinned open-data sources into the cache",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return newApp(cmd).Init(cmd.Context())
			},
		},
		&cobra.Command{
			Use:   "build [id...]",
			Short: "Build data/ for all overrides, or only the given ids",
			RunE: func(cmd *cobra.Command, args []string) error {
				return newApp(cmd).Build(cmd.Context(), args...)
			},
		},
		&cobra.Command{
			Use:   "refresh",
			Short: "Update sources to latest, bump pins, and rebuild everything",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return newApp(cmd).Refresh(cmd.Context())
			},
		},
	)
	return root
}
