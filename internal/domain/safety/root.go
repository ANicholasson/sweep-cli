package safety

import "strings"

// Beneath reports whether path is beneath root and no Link lies on the way.
// link is true when a Link was crossed to reach path, and then the answer is
// no even when the text of path sits under root.
//
// A Scan calls it for every path it would enter or report. A Clean calls
// the same function again before it touches a path.
//
// A drive letter and a UNC host and share compare without letter case.
// Every other part compares with it, so the check means the same on Linux,
// where letter case is a different path. ".." that leaves root answers no.
// ".." does not climb off a volume.
func Beneath(root, path string, link bool) bool {
	if link {
		return false
	}
	rv, rp, rok := parse(root)
	pv, pp, pok := parse(path)
	if !rok || !pok || rv != pv || len(pp) < len(rp) {
		return false
	}
	for i := range rp {
		if rp[i] != pp[i] {
			return false
		}
	}
	return true
}

func parse(p string) (vol string, parts []string, ok bool) {
	if p == "" || strings.IndexByte(p, 0) >= 0 {
		return "", nil, false
	}
	p = strings.ReplaceAll(p, `\`, `/`)
	if len(p) >= 4 && strings.EqualFold(p[:4], "//?/") {
		p = p[4:]
		if len(p) >= 4 && strings.EqualFold(p[:4], "unc/") {
			p = "//" + p[4:]
		}
	}
	switch {
	case strings.HasPrefix(p, "//"):
		chunks := strings.Split(strings.TrimPrefix(p, "//"), "/")
		if len(chunks) < 2 || chunks[0] == "" || chunks[1] == "" {
			return "", nil, false
		}
		vol = "//" + strings.ToLower(chunks[0]) + "/" + strings.ToLower(chunks[1])
		return vol, clean(chunks[2:]), true
	case len(p) >= 3 && letter(p[0]) && p[1] == ':' && p[2] == '/':
		return strings.ToLower(p[:2]), clean(strings.Split(p[3:], "/")), true
	case strings.HasPrefix(p, "/"):
		return "", clean(strings.Split(p, "/")), true
	default:
		return "", nil, false
	}
}

func clean(raw []string) []string {
	parts := make([]string, 0, len(raw))
	for _, s := range raw {
		switch s {
		case "", ".":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, s)
		}
	}
	return parts
}

func letter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
