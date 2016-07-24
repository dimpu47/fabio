package logger

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	fields := map[string]field{
		"$a": func(b *bytes.Buffer, _, _ time.Time, _ *http.Response, _ *http.Request) {
			b.WriteString("aa")
		},
		"$b": func(b *bytes.Buffer, _, _ time.Time, _ *http.Response, _ *http.Request) {
			b.WriteString("bb")
		},
	}
	req := &http.Request{
		Header: http.Header{
			"User-Agent":      {"Mozilla Firefox"},
			"X-Forwarded-For": {"3.3.3.3"},
		},
	}
	tests := []struct {
		format string
		out    string
	}{
		{"", ""},
		{"$a", "aa\n"},
		{"$a $b", "aa bb\n"},
		{"$a \"$header.User-Agent\"", "aa \"Mozilla Firefox\"\n"},
	}

	for i, tt := range tests {
		p, err := parse(tt.format, fields)
		if err != nil {
			t.Errorf("%d: got %v want nil", i, err)
			continue
		}
		var b bytes.Buffer
		p.write(&b, time.Time{}, time.Time{}, nil, req)
		if got, want := string(b.Bytes()), tt.out; got != want {
			t.Errorf("%d: got %q want %q", i, got, want)
		}
	}
}

func TestLog(t *testing.T) {
	t1 := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(123456789 * time.Nanosecond)

	req := &http.Request{
		RequestURI: "/?q=x",
		Header: http.Header{
			"User-Agent":      {"Mozilla Firefox"},
			"Referer":         {"http://foo.com/"},
			"X-Forwarded-For": {"3.3.3.3"},
		},
		RemoteAddr: "2.2.2.2:666",
		Host:       "foo.com",
		URL: &url.URL{
			Path:     "/",
			RawQuery: "?q=x",
			Host:     "proxy host",
		},
		Method: "GET",
		Proto:  "HTTP/1.1",
	}

	resp := &http.Response{
		StatusCode:    200,
		ContentLength: 1234,
		Header:        http.Header{"foo": []string{"bar"}},
		Request: &http.Request{
			RemoteAddr: "5.6.7.8:1234",
		},
	}

	tests := []struct {
		format string
		out    string
	}{
		{"$header.Referer", "http://foo.com/\n"},
		{"$header.user-agent", "Mozilla Firefox\n"},
		{"$header.X-Forwarded-For", "3.3.3.3\n"},
		{"$request", "GET /?q=x HTTP/1.1\n"},
		{"$request_args", "?q=x\n"},
		{"$request_host", "foo.com\n"}, // TODO(fs): is this correct?
		{"$request_method", "GET\n"},
		{"$request_uri", "/?q=x\n"},
		{"$request_proto", "HTTP/1.1\n"},
		{"$remote_addr", "2.2.2.2:666\n"},
		{"$remote_host", "2.2.2.2\n"},
		{"$remote_port", "666\n"},
		{"$response_body_size", "1234\n"},
		{"$response_status", "200\n"},
		{"$response_time_ms", "0.123\n"},       // TODO(fs): is this correct?
		{"$response_time_us", "0.123456\n"},    // TODO(fs): is this correct?
		{"$response_time_ns", "0.123456789\n"}, // TODO(fs): is this correct?
		{"$time_rfc3339", "2016-01-01T00:00:00Z\n"},
		{"$time_rfc3339_ms", "2016-01-01T00:00:00.123Z\n"},
		{"$time_rfc3339_us", "2016-01-01T00:00:00.123456Z\n"},
		{"$time_rfc3339_ns", "2016-01-01T00:00:00.123456789Z\n"},
		{"$time_unix_ms", "1451606400123\n"},
		{"$time_unix_us", "1451606400123456\n"},
		{"$time_unix_ns", "1451606400123456789\n"},
		{"$time_common", "01/Jan/2016:00:00:00 +0000\n"},
		{"$upstream_addr", "5.6.7.8:1234\n"}, // TODO(fs): is this correct?
		{"$upstream_host", "5.6.7.8\n"},      // TODO(fs): is this correct?
		{"$upstream_port", "1234\n"},         // TODO(fs): is this correct?
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			b := new(bytes.Buffer)

			l, err := New(b, tt.format)
			if err != nil {
				t.Fatalf("got %v want nil", err)
			}

			l.Log(t1, t2, resp, req)
			if got, want := string(b.Bytes()), tt.out; got != want {
				t.Errorf("got %q want %q", got, want)
			}
		})
	}
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		i   int64
		pad int
		s   string
	}{
		{i: 0, pad: 0, s: "0"},
		{i: 1, pad: 0, s: "1"},
		{i: -1, pad: 0, s: "-1"},
		{i: 12345, pad: 0, s: "12345"},
		{i: -12345, pad: 0, s: "-12345"},
		{i: 9223372036854775807, pad: 0, s: "9223372036854775807"},
		{i: -9223372036854775807, pad: 0, s: "-9223372036854775807"},

		{i: 0, pad: 5, s: "00000"},
		{i: 1, pad: 5, s: "00001"},
		{i: -1, pad: 5, s: "-00001"},
		{i: 12345, pad: 5, s: "12345"},
		{i: -12345, pad: 5, s: "-12345"},
		{i: 9223372036854775807, pad: 5, s: "9223372036854775807"},
		{i: -9223372036854775807, pad: 5, s: "-9223372036854775807"},
	}

	for i, tt := range tests {
		var b bytes.Buffer
		atoi(&b, tt.i, tt.pad)
		if got, want := string(b.Bytes()), tt.s; got != want {
			t.Errorf("%d: got %q want %q", i, got, want)
		}
	}
}

func BenchmarkLog(b *testing.B) {
	t1 := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(100 * time.Millisecond)
	req := &http.Request{
		RequestURI: "/?q=x",
		Header: http.Header{
			"User-Agent":      {"Mozilla Firefox"},
			"Referer":         {"http://foo.com/"},
			"X-Forwarded-For": {"3.3.3.3"},
		},
		RemoteAddr: "2.2.2.2:666",
		Host:       "foo.com",
		URL: &url.URL{
			Path:     "/",
			RawQuery: "?q=x",
			Host:     "proxy host",
		},
		Method: "GET",
		Proto:  "HTTP/1.1",
	}

	resp := &http.Response{
		StatusCode:    200,
		ContentLength: 1234,
		Header:        http.Header{"foo": []string{"bar"}},
		Request: &http.Request{
			RemoteAddr: "5.6.7.8:1234",
		},
	}

	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	format := strings.Join(keys, " ")

	l, err := New(ioutil.Discard, format)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < b.N; i++ {
		l.Log(t1, t2, resp, req)
	}
}
