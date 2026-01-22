package main

import (
	"encoding/base64"
	"fmt"
	"strings"

	log "github.com/schollz/logger"
)

func fromURI(u string) (st, ne, as, a6, ps, pd, s5, ct string, err error) {
	if len(u) <= len(IO) || !strings.HasPrefix(u, IO) {
		err = fmt.Errorf("not IO")
		return
	}

	base64Str := strings.TrimPrefix(u, IO)
	decoded, err := base64.RawURLEncoding.DecodeString(base64Str)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(base64Str)
		if err != nil {
			return
		}
	}

	str := strings.TrimRight(string(decoded), "\n")
	ss := strings.Split(str, "\n")
	for i, s := range ss {
		switch i {
		case 0:
			st = s
		case 1:
			ne = s
		case 2:
			as = s
		case 3:
			a6 = s
		case 4:
			ps = s
		case 5:
			pd = s
		case 6:
			s5 = s
		case 7:
			ct = s
		default:
			return
		}
	}
	log.Debug("st, ne, as, a6, ps, pd, s5, ct")
	log.Debug("%v", ss)

	return
}

// st, ne, as, a6, ps, pd, s5, ct
func toURI(ss ...string) (u string) {
	for i, s := range ss {
		if i > 7 {
			break
		}
		s = strings.ReplaceAll(s, "\n", "")
		u += strings.TrimSpace(s) + "\n"
	}
	u = strings.TrimRight(u, "\n")
	log.Debug(IO + u)
	return IO + base64.RawURLEncoding.EncodeToString([]byte(u))
}
