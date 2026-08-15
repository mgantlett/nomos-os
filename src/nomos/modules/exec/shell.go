package exec

import "strings"

// ShellEscapeArgs formats an executable name and its arguments array into a single
// shell-escaped command string suitable for execution in a shell environment.
func ShellEscapeArgs(name string, args []string) string {
	var parts []string
	parts = append(parts, Quote(name))
	for _, arg := range args {
		parts = append(parts, Quote(arg))
	}
	return strings.Join(parts, " ")
}

// Quote wraps a string in single quotes and escapes any single quotes inside it
// using standard POSIX single-quote escaping syntax.
// This is critical to ensure that arguments containing single quotes do not break
// the outer single-quoted boundaries when passed to nix-shell or raw bash execution.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
