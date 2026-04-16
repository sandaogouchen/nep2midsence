package cli

import (
	"strings"

	"github.com/sandaogouchen/nep2midsence/internal/cli/commands"
	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// REPLCommand interface that all commands implement.
type REPLCommand interface {
	Name() string
	Aliases() []string
	Description() string
	Usage() string
	Execute(args []string, cfg *config.Config) error
}

// CommandRegistry holds all registered commands.
type CommandRegistry struct {
	commands []REPLCommand
	cfg      *config.Config
}

// NewCommandRegistry creates a registry and registers all 16 built-in commands.
func NewCommandRegistry(cfg *config.Config) *CommandRegistry {
	r := &CommandRegistry{
		commands: make([]REPLCommand, 0, 16),
		cfg:      cfg,
	}

	// We register help first so its index is 0; it needs back-reference.
	helpCmd := commands.NewHelpCommand(nil) // placeholder, patched below
	r.Register(helpCmd)
	r.Register(commands.NewStartCommand())
	r.Register(commands.NewAnalyzeCommand())
	r.Register(commands.NewConfigCommand())
	r.Register(commands.NewStatusCommand())
	r.Register(commands.NewHistoryCommand())
	r.Register(commands.NewResumeCommand())
	r.Register(commands.NewDryRunCommand())
	r.Register(commands.NewMappingCommand())
	r.Register(commands.NewVersionCommand())
	r.Register(commands.NewPreviewCommand())
	r.Register(commands.NewRollbackCommand())
	r.Register(commands.NewClearCommand())
	r.Register(commands.NewExportCommand())
	r.Register(commands.NewValidateCommand())
	r.Register(commands.NewQuitCommand())

	// Patch help command with the full command list provider.
	helpCmd.SetListFunc(func() []commands.CommandInfo {
		infos := make([]commands.CommandInfo, 0, len(r.commands))
		for _, c := range r.commands {
			infos = append(infos, commands.CommandInfo{
				Name:        c.Name(),
				Aliases:     c.Aliases(),
				Description: c.Description(),
				Usage:       c.Usage(),
			})
		}
		return infos
	})

	return r
}

// Register adds a command to the registry.
func (r *CommandRegistry) Register(cmd REPLCommand) {
	r.commands = append(r.commands, cmd)
}

// Find looks up a command by name or alias (case-insensitive).
func (r *CommandRegistry) Find(name string) REPLCommand {
	lower := strings.ToLower(name)
	for _, cmd := range r.commands {
		if strings.ToLower(cmd.Name()) == lower {
			return cmd
		}
		for _, alias := range cmd.Aliases() {
			if strings.ToLower(alias) == lower {
				return cmd
			}
		}
	}
	return nil
}

// All returns every registered command.
func (r *CommandRegistry) All() []REPLCommand {
	return r.commands
}
