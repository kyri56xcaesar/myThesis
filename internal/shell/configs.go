package shell

const (
	shell_sign string = "k_s>"
	delimitter string = "@"

	Reset   string = "\033[0m"
	Red     string = "\033[31m"
	Green   string = "\033[32m"
	Yellow  string = "\033[33m"
	Blue    string = "\033[34m"
	Magenta string = "\033[35m"
	Cyan    string = "\033[36m"
	Gray    string = "\033[37m"
	White   string = "\033[97m"
)

var (
	colorred  bool   = true
	usercolor string = White
	hostcolor string = White
	signcolor string = White
)

type SConfig struct {
}

func ColorText(s string, color string) string {
	return "" + color + s + Reset
}
