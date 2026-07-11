package targets

import "strings"

func Parse(value string) []string {
	targets := make([]string, 0, 4)
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			targets = append(targets, part)
		}
	}
	return targets
}

func Resolve(targets []string, gateway string) []string {
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		resolved := ResolveOne(target, gateway)
		if resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

func ResolveOne(target, gateway string) string {
	target = strings.TrimSpace(target)
	switch strings.ToLower(target) {
	case "localhost":
		return "127.0.0.1"
	case "gateway":
		return strings.TrimSpace(gateway)
	default:
		return target
	}
}

func NeedsGateway(targets []string) bool {
	for _, target := range targets {
		if strings.EqualFold(strings.TrimSpace(target), "gateway") {
			return true
		}
	}
	return false
}
