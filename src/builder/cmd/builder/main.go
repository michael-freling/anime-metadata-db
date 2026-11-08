// Command builder compiles the authored override layer plus pinned open-data
// sources into the generated data/ dataset.
//
// Usage:
//
//	builder init                 download the pinned sources into the cache
//	builder build [id...]        (re)build all overrides, or just the given ids
//	builder refresh              update sources to latest + rebuild everything
//	builder import --year YYYY   write override files for one year of upstream
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/builder"
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

	// The one destructive flag. It sits on the two commands that actually
	// build — refresh runs the same build and hits the same refusal, so
	// omitting it there leaves that workflow with no way to complete an
	// intended removal.
	var allowPrune bool
	build := &cobra.Command{
		Use:   "build [id...]",
		Short: "Build data/ for all overrides, or only the given ids",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := newApp(cmd)
			app.AllowPrune = allowPrune
			return app.Build(cmd.Context(), args...)
		},
	}
	refresh := &cobra.Command{
		Use:   "refresh",
		Short: "Update sources to latest, bump pins, and rebuild everything",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := newApp(cmd)
			app.AllowPrune = allowPrune
			return app.Refresh(cmd.Context())
		},
	}
	for _, c := range []*cobra.Command{build, refresh} {
		c.Flags().BoolVar(&allowPrune, "allow-prune", false,
			"permit deleting more records than the build keeps (refused by default, "+
				"since that shape usually means the wrong overrides directory)")
	}

	var importYear int
	var importDryRun bool
	imp := &cobra.Command{
		Use:   "import",
		Short: "Write override files for one year of the upstream catalogue",
		Long: "Write override files for one year of the upstream catalogue.\n\n" +
			"Writes the authored layer only and never builds, so the result is " +
			"reviewable as a diff of overrides before any record is generated " +
			"from it. Works belonging to a series the catalogue already has are " +
			"reported rather than written: creating a second series for an " +
			"installment of an existing one is the one mistake a later import " +
			"cannot detect.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if importYear == 0 {
				return fmt.Errorf("--year is required: the airing year to import")
			}
			return newApp(cmd).Import(importYear, importDryRun)
		},
	}
	imp.Flags().IntVar(&importYear, "year", 0, "airing year to import")
	imp.Flags().BoolVar(&importDryRun, "dry-run", false, "report what would be written, write nothing")

	var diffBase string
	diff := &cobra.Command{
		Use:   "diff",
		Short: "Compare the built dataset against another built copy of it",
		Long: "Compare the built dataset against another built copy of it.\n\n" +
			"--base names a data directory built from the branch being compared to, " +
			"not a git ref: the comparison is over built output, so what the overrides " +
			"and the builder would produce is what is compared, rather than whatever " +
			"happens to be committed.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if diffBase == "" {
				return fmt.Errorf("--base is required: the data directory to compare against")
			}
			return newApp(cmd).Diff(diffBase)
		},
	}
	diff.Flags().StringVar(&diffBase, "base", "", "data directory built from the base branch")

	root.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: "Download the pinned open-data sources into the cache",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return newApp(cmd).Init(cmd.Context())
			},
		},
		build,
		refresh,
		diff,
		imp,
	)
	return root
}
