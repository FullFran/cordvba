package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/cli"
)

// The daemon must poll everything shortly after startup and shut down cleanly
// when its context ends. A daemon that ignores cancellation is a daemon you
// have to kill.
func TestDaemonPollsThenStopsOnCancellation(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	dataDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	code := cli.New().Run(ctx,
		[]string{"daemon", "--registry", registry, "--data-dir", dataDir},
		&stdout, &stderr)

	if code != 0 {
		t.Errorf("exit = %d, want 0 on a clean shutdown: %s", code, stderr.String())
	}

	logs := stderr.String()
	if !strings.Contains(logs, `"msg":"starting"`) {
		t.Errorf("daemon did not log startup:\n%s", logs)
	}
	if !strings.Contains(logs, `"msg":"stopped"`) {
		t.Errorf("daemon did not log a clean stop:\n%s", logs)
	}
	// Every source must have been polled immediately, not after its own
	// interval: a twelve-hour source should not make an operator wait to
	// find out whether it works at all.
	if !strings.Contains(logs, `"msg":"initial pass complete"`) {
		t.Errorf("the daemon did not poll on startup:\n%s", logs)
	}
	if !strings.Contains(logs, `"answered":4`) {
		t.Errorf("the initial pass did not reach every live source:\n%s", logs)
	}

	// What it collected must be readable afterwards.
	_, out, _ := runCmdIn(t, registry, dataDir, "news", "--offline")
	if !strings.Contains(out, "Puente Romano") {
		t.Errorf("the daemon stored nothing readable:\n%s", out)
	}
}

// Each prune cycle logs both what it removed and what remains, so an
// operator watching the raw cache grow does not have to infer it from a full
// disk later.
func TestDaemonLogsRetentionEachPruneCycle(t *testing.T) {
	t.Parallel()

	registry := setup(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cli.New().Run(ctx,
		[]string{"daemon", "--registry", registry, "--data-dir", t.TempDir(), "--prune-every", "200ms"},
		&stdout, &stderr)

	logs := stderr.String()
	if !strings.Contains(logs, `"msg":"retention enforced"`) {
		t.Fatalf("no prune cycle logged within the run:\n%s", logs)
	}
	for _, want := range []string{
		`"expired_records"`, `"payloads_removed"`, `"payload_bytes_freed"`, `"store_bytes"`, `"raw_cache_bytes"`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("prune log is missing %s:\n%s", want, logs)
		}
	}
}

func TestDaemonLogsAreStructuredJSON(t *testing.T) {
	t.Parallel()

	registry := setup(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cli.New().Run(ctx,
		[]string{"daemon", "--registry", registry, "--data-dir", t.TempDir()},
		&stdout, &stderr)

	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		if _, ok := entry["msg"]; !ok {
			t.Errorf("log entry has no msg: %v", entry)
		}
	}
}

func TestDaemonRefusesWithNoLiveSource(t *testing.T) {
	t.Parallel()

	// A registry whose only entries are held or unreadable.
	registry := writeHeldOnlyRegistry(t)

	code, _, stderr := runCmd(t, registry, "daemon", "--once")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no live source") {
		t.Errorf("stderr = %q", stderr)
	}
}

// writeHeldOnlyRegistry writes a registry with nothing eye may and can poll.
func writeHeldOnlyRegistry(t *testing.T) string {
	t.Helper()

	return writeRegistryBody(t, `
sources:
  - id: held
    authority: Fuente Retenida
    topic: hydrology
    url: https://example.org/held
    format: web
    license: unspecified
    access: public_html
    automation: review_terms

  - id: unreadable
    authority: Fuente Sin Adaptador
    topic: weather
    url: https://example.org/wms
    format: wms
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 1h
`)
}
