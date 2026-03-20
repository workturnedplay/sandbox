package selfcheck

type State int

const (
	StateVerified State = iota
	StateUninitialized
	StateTainted
	StateUnknown
)

func VerifyAtStartup() (State, error)
func PatchExecutable(path string) error
func StateValue() State

