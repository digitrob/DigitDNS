package tui

// ANSI sequences shared by the console surfaces. The old interactive manager
// declared these alongside its menu; the uninstaller and the launcher still
// colour their output with them, so they live on their own now that the menu is
// gone.
const (
	Cyan   = "\033[36m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Red    = "\033[31m"
	Bold   = "\033[1m"
	Reset  = "\033[0m"
)
