//go:build !windows

package killswitch

import "errors"

type Options struct {
	AllowedPrograms []string
	BlockLAN        bool
	TUNSubnets      []string
}

type RuleSpec struct {
	Name      string
	Program   string
	LocalIP   string
	RemoteIP  string
	Protocol  string
	LocalPort string
}

func BuildRules(opts Options) []RuleSpec { return nil }
func Enable(opts Options) error          { return errors.New("kill switch yalnız Windows'ta desteklenir") }
func Disable() error                     { return nil }
func IsActive() bool                     { return false }
func DefaultWarpServiceExe() string      { return "" }
