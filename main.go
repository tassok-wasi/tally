package main

import (
	"github.com/alecthomas/kong"
)

const version = "tally 1.0.0"

// CLI is the root command. Running it with no subcommand counts lines/files by
// extension in the current directory (or -d); the other commands are
// subcommands.
type CLI struct {
	ScanFlags
	Count     CountCmd         `cmd:"" help:"Count lines/files by extension (default)."`
	Diff      DiffCmd          `cmd:"" help:"Compare two directories and show per-key line deltas."`
	Check     CheckCmd         `cmd:"" help:"Enforce line/file budgets; exits non-zero when exceeded."`
	Init      InitCmd          `cmd:"" help:"Write a default config file."`
	Languages LanguagesCmd     `cmd:"" help:"List supported comment syntaxes."`
	Version   kong.VersionFlag `help:"Print version and exit."`
}

// Run implements the default (no subcommand) behaviour: count in the current
// directory. When a subcommand is selected, kong also invokes this ancestor
// Run, so it must be a no-op in that case.
func (c *CLI) Run(ctx *kong.Context) error {
	if ctx.Selected() != nil {
		return nil
	}
	root := c.Dir
	if root == "" {
		root = "."
	}
	return runCount(c.ScanFlags, root)
}

func main() {
	cli := CLI{}
	ctx := kong.Parse(&cli,
		kong.Name("tally"),
		kong.Description("Count lines of code grouped by file extension."),
		kong.Vars{"version": version},
		kong.Configuration(kong.JSON, "tally.json", "~/.config/tally/config.json"),
	)
	// Subcommands share the root's global scan flags.
	cli.Count.scan = &cli.ScanFlags
	cli.Diff.scan = &cli.ScanFlags
	cli.Check.scan = &cli.ScanFlags
	ctx.FatalIfErrorf(ctx.Run())
}
