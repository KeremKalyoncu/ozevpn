//go:build !windows

package tray

type Options struct {
	GetState         func() (connected bool, tooltip string)
	OnOpenPanel      func()
	OnToggleConnect  func()
	OnEmergencyReset func()
	OnQuit           func()
}

type Tray struct{}

func Start(opts Options) *Tray { return &Tray{} }
func (t *Tray) Stop()          {}
