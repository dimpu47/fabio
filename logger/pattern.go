package logger

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"time"
)

// field renders a part of the log line.
type field func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request)

// Pattern is a log output format.
type pattern []field

func (p pattern) write(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
	for _, fn := range p {
		fn(b, t1, t2, w, r)
	}
	if b.Len() == 0 {
		return
	}
	b.WriteRune('\n')
}

// parse parses a format string into a pattern based on the following rules:
//
// The format string consists of text and fields. Field names start with a '$'
// and consist of ASCII characters [a-zA-Z0-9.-_]. Field names like
// '$header.name' will render the HTTP header 'name'. All other field names
// must exist in the fields map.
func parse(format string, fields map[string]field) (p pattern, err error) {
	// text is a helper to add raw text to the log output.
	text := func(s string) field {
		return func(b *bytes.Buffer, _, _ time.Time, _ *http.Response, _ *http.Request) {
			b.WriteString(s)
		}
	}

	// header is a helper to add an HTTP header to the log output.
	header := func(name string) field {
		return func(b *bytes.Buffer, _, _ time.Time, _ *http.Response, r *http.Request) {
			b.WriteString(r.Header.Get(name))
		}
	}

	s := []rune(format)
	for {
		if len(s) == 0 {
			break
		}
		typ, n := lex(s)
		val := string(s[:n])
		s = s[n:]
		switch typ {
		case itemText:
			p = append(p, text(val))
		case itemHeader:
			p = append(p, header(val[len("$header."):]))
		case itemField:
			f := fields[val]
			if f == nil {
				return nil, fmt.Errorf("invalid field %q", val)
			}
			p = append(p, f)
		}
	}
	return p, nil
}

// fields contains the known log fields and their field functions. The field
// functions should avoid to alloc memory at all cost since they are in the hot
// path. Do not use fmt.Sprintf() but combine the value from the parts. Instead
// of strconv.Atoi/FormatInt() use the local atoi() function which does not
// alloc.
var shortMonthNames = []string{
	"---",
	"Jan",
	"Feb",
	"Mar",
	"Apr",
	"May",
	"Jun",
	"Jul",
	"Aug",
	"Sep",
	"Oct",
	"Nov",
	"Dec",
}

var fields = map[string]field{
	"$remote_addr": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.RemoteAddr)
	},
	"$remote_host": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		b.WriteString(host)
	},
	"$remote_port": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		_, port, _ := net.SplitHostPort(r.RemoteAddr)
		b.WriteString(port)
	},
	"$request": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.Method)
		b.WriteRune(' ')
		b.WriteString(r.RequestURI)
		b.WriteRune(' ')
		b.WriteString(r.Proto)
	},
	"$request_args": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.URL.RawQuery)
	},
	"$request_host": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.Host)
	},
	"$request_method": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.Method)
	},
	"$request_uri": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.RequestURI)
	},
	"$request_proto": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(r.Proto)
	},
	"$response_body_size": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, w.ContentLength, 0)
	},
	"$response_status": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(w.StatusCode), 0)
	},
	"$response_time_ms": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		d := t2.Sub(t1).Nanoseconds()
		s, µs := d/int64(time.Second), d%int64(time.Second)/int64(time.Millisecond)
		atoi(b, s, 0)
		b.WriteRune('.')
		atoi(b, µs, 3)
	},
	"$response_time_us": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		d := t2.Sub(t1).Nanoseconds()
		s, µs := d/int64(time.Second), d%int64(time.Second)/int64(time.Microsecond)
		atoi(b, s, 0)
		b.WriteRune('.')
		atoi(b, µs, 6)
	},
	"$response_time_ns": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		d := t2.Sub(t1).Nanoseconds()
		s, ns := d/int64(time.Second), d%int64(time.Second)/int64(time.Nanosecond)
		atoi(b, s, 0)
		b.WriteRune('.')
		atoi(b, ns, 9)
	},
	"$time_unix_ms": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, t2.UnixNano()/int64(time.Millisecond), 0)
	},
	"$time_unix_us": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, t2.UnixNano()/int64(time.Microsecond), 0)
	},
	"$time_unix_ns": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, t2.UnixNano(), 0)
	},
	"$time_common": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(t2.Day()), 2)
		b.WriteRune('/')
		b.WriteString(shortMonthNames[t2.Month()])
		b.WriteRune('/')
		atoi(b, int64(t2.Year()), 4)
		b.WriteRune(':')
		atoi(b, int64(t2.Hour()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Minute()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Second()), 2)
		b.WriteString(" +0000") // TODO(fs): local time
	},
	"$time_rfc3339": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(t2.Year()), 4)
		b.WriteRune('-')
		atoi(b, int64(t2.Month()), 2)
		b.WriteRune('-')
		atoi(b, int64(t2.Day()), 2)
		b.WriteRune('T')
		atoi(b, int64(t2.Hour()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Minute()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Second()), 2)
		b.WriteRune('Z')
	},
	"$time_rfc3339_ms": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(t2.Year()), 4)
		b.WriteRune('-')
		atoi(b, int64(t2.Month()), 2)
		b.WriteRune('-')
		atoi(b, int64(t2.Day()), 2)
		b.WriteRune('T')
		atoi(b, int64(t2.Hour()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Minute()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Second()), 2)
		b.WriteRune('.')
		atoi(b, int64(t2.Nanosecond())/int64(time.Millisecond), 3)
		b.WriteRune('Z')
	},
	"$time_rfc3339_us": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(t2.Year()), 4)
		b.WriteRune('-')
		atoi(b, int64(t2.Month()), 2)
		b.WriteRune('-')
		atoi(b, int64(t2.Day()), 2)
		b.WriteRune('T')
		atoi(b, int64(t2.Hour()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Minute()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Second()), 2)
		b.WriteRune('.')
		atoi(b, int64(t2.Nanosecond())/int64(time.Microsecond), 6)
		b.WriteRune('Z')
	},
	"$time_rfc3339_ns": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		atoi(b, int64(t2.Year()), 4)
		b.WriteRune('-')
		atoi(b, int64(t2.Month()), 2)
		b.WriteRune('-')
		atoi(b, int64(t2.Day()), 2)
		b.WriteRune('T')
		atoi(b, int64(t2.Hour()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Minute()), 2)
		b.WriteRune(':')
		atoi(b, int64(t2.Second()), 2)
		b.WriteRune('.')
		atoi(b, int64(t2.Nanosecond()), 9)
		b.WriteRune('Z')
	},
	"$upstream_addr": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		b.WriteString(w.Request.RemoteAddr)
	},
	"$upstream_host": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		host, _, _ := net.SplitHostPort(w.Request.RemoteAddr)
		b.WriteString(host)
	},
	"$upstream_port": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
		_, port, _ := net.SplitHostPort(w.Request.RemoteAddr)
		b.WriteString(port)
	},

	//"$http_referer": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
	//	b.WriteString(r.Referer())
	//},
	//"$http_user_agent": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
	//	b.WriteString(r.UserAgent())
	//},
	//"$http_x_forwarded_for": func(b *bytes.Buffer, t1, t2 time.Time, w *http.Response, r *http.Request) {
	//	b.WriteString(r.Header.Get("X-Forwarded-For"))
	//},
}

type itemType int

const (
	itemText itemType = iota
	itemField
	itemHeader
)

func (t itemType) String() string {
	switch t {
	case itemText:
		return "TEXT"
	case itemField:
		return "FIELD"
	case itemHeader:
		return "HEADER"
	}
	panic("invalid")
}

type state int

const (
	stateStart state = iota
	stateText
	stateDollar
	stateField
	stateDot
	stateHeader
)

func lex(s []rune) (typ itemType, n int) {
	isIDChar := func(r rune) bool {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9' || r == '_' || r == '-'
	}

	state := stateStart
	for i, r := range s {
		switch state {
		case stateStart:
			switch r {
			case '$':
				state = stateDollar
			default:
				state = stateText
			}

		case stateText:
			switch r {
			case '$':
				return itemText, i
			default:
				// state = stateText
			}

		case stateDollar:
			switch {
			case isIDChar(r):
				state = stateField
			default:
				state = stateText
			}

		case stateField:
			switch {
			case r == '.':
				if string(s[:i]) == "$header" {
					state = stateDot
				} else {
					return itemField, i
				}
			case isIDChar(r):
				// state = stateField
			default:
				return itemField, i
			}

		case stateDot:
			switch {
			case isIDChar(r):
				state = stateHeader
			default:
				return itemField, i
			}

		case stateHeader:
			switch {
			case isIDChar(r):
				// state = stateHeader
			default:
				return itemHeader, i
			}
		}
	}

	switch state {
	case stateDot:
		return itemField, len(s) - 1
	case stateField:
		return itemField, len(s)
	case stateHeader:
		return itemHeader, len(s)
	default:
		return itemText, len(s)
	}
}
