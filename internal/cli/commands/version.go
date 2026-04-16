package commands

import (
	"fmt"
	"runtime"

	"github.com/sandaogouchen/nep2midsence/internal/config"
)

// Version constants. In production these are set via ldflags at build time.
var (
	AppVersion = "2.0.0"
	GitCommit  = "unknown"
	BuildDate  = "unknown"
)

// VersionCommand implements /version (alias /v).
type VersionCommand struct{}

func NewVersionCommand() *VersionCommand { return &VersionCommand{} }

func (c *VersionCommand) Name() string        { return "version" }
func (c *VersionCommand) Aliases() []string    { return []string{"v"} }
func (c *VersionCommand) Description() string  { return "显示版本信息" }
func (c *VersionCommand) Usage() string        { return "/version" }

func (c *VersionCommand) Execute(args []string, cfg *config.Config) error {
	fmt.Printf("nep2midsence v%s\n", AppVersion)
	fmt.Printf("  Git Commit: %s\n", GitCommit)
	fmt.Printf("  Build Date: %s\n", BuildDate)
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return nil
}
