package sentinel

import (
	"os"
	"strings"
	"testing"
)

func TestTomlStrEscaping(t *testing.T) {
	cases := map[string]string{
		`plain`:            `"plain"`,
		`with"quote`:       `"with\"quote"`,
		`back\slash`:       `"back\\slash"`,
		`https://x.pt/api`: `"https://x.pt/api"`,
	}
	for in, want := range cases {
		if got := tomlStr(in); got != want {
			t.Errorf("tomlStr(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestRenderAgentConfig(t *testing.T) {
	resp := &registerResponse{
		AgentID:                  "ag_123",
		AgentToken:               "at_abc-DEF_456",
		CollectIntervalSeconds:   1,
		FlushIntervalSeconds:     30,
		PollLongWaitSeconds:      30,
		HeartbeatIntervalSeconds: 30,
	}
	out := renderAgentConfig("https://sentinel.erp24.pt", resp)

	// The two keys the agent's config.Load hard-requires must be present and
	// non-empty, plus the section headers it unmarshals.
	mustContain := []string{
		"[central]",
		`url = "https://sentinel.erp24.pt"`,
		`agent_id = "ag_123"`,
		`agent_token = "at_abc-DEF_456"`,
		"verify_tls = true",
		"[intervals]",
		"collect_seconds = 1",
		"heartbeat_seconds = 30",
		"[buffer]",
		`backend = "sqlite"`,
		"[[docker]]",
		`socket = "/var/run/docker.sock"`,
		"[host]",
		"collect_per_core = true",
		"[updates]",
		"check_interval_seconds = 600",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("rendered agent.toml missing %q\n---\n%s", s, out)
		}
	}

	// Optional: dump the exact output so an external go-toml harness can prove
	// it parses against the agent's own schema. No-op in normal test runs.
	if p := os.Getenv("SENTINEL_TOML_OUT"); p != "" {
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			t.Fatalf("dump: %v", err)
		}
	}
}
