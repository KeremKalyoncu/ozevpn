//go:build !windows

package tray

type VisualState int

const (
	StateDisconnected VisualState = iota
	StateConnecting
	StateConnected
)

type Options struct {
	GetState         func() (connected bool, tooltip string)
	GetVisualState   func() (state VisualState, tooltip string)
	OnOpenPanel      func()
	OnToggleConnect  func()
	OnEmergencyReset func()
	OnPowerResume    func()
	OnQuit           func()
}

type Tray struct{}

func Start(opts Options) *Tray { return &Tray{} }
func (t *Tray) Stop()          {}
