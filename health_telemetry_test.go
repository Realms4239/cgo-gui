package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestTelemetryLabelGUI — trois états seulement : ebpf, tcpinfo, le reste.
func TestTelemetryLabelGUI(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{"ebpf", "eBPF"},
		{"tcpinfo", "TCP_INFO"},
		{"", "non instrumenté"},
		{"autre", "non instrumenté"},
	}
	for _, tc := range cases {
		if got := telemetryLabelGUI(tc.source); got != tc.want {
			t.Errorf("telemetryLabelGUI(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

// TestDashGetCarriesTelemetry — dashGet rend version + palier de télémétrie
// tels que /api/health les expose ; absent, refusé ou illisible → pas de KO.
func TestDashGetCarriesTelemetry(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{
			name: "tcpinfo avec raison",
			body: `{"ok":true,"version":"1.3.5","telemetry":{"source":"tcpinfo","reason":"hôte non-Linux"}}`,
			want: "1.3.5 · télémétrie TCP_INFO",
			ok:   true,
		},
		{
			name: "ebpf",
			body: `{"ok":true,"version":"1.3.5","telemetry":{"source":"ebpf"}}`,
			want: "1.3.5 · télémétrie eBPF",
			ok:   true,
		},
		{
			name: "télémétrie absente",
			body: `{"ok":true,"version":"1.3.5"}`,
			want: "1.3.5 · télémétrie non instrumenté",
			ok:   true,
		},
		{
			name: "source inconnue",
			body: `{"ok":true,"version":"1.3.5","telemetry":{"source":"rttmonitor"}}`,
			want: "1.3.5 · télémétrie non instrumenté",
			ok:   true,
		},
		{
			name: "version vide",
			body: `{"ok":true,"telemetry":{"source":"tcpinfo"}}`,
			want: "version inconnue · télémétrie TCP_INFO",
			ok:   true,
		},
		{
			name: "ok faux",
			body: `{"ok":false}`,
			want: "",
			ok:   false,
		},
		{
			name: "corps illisible",
			body: `not json`,
			want: "",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			got, ok := dashGet(&http.Client{Timeout: 2 * time.Second}, srv.URL)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("dashGet = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
