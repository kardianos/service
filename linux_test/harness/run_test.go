package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeRuntime gives a scripted boot, test list and test result. Thus the tests
// do not need podman.
type fakeRuntime struct {
	buildErr error
	ready    bool
	list     string
	testExit int
	testErr  error

	builds  int
	started []string
	removed []string
	execs   [][]string
}

func (f *fakeRuntime) Info(context.Context) (HostInfo, error)               { return HostInfo{"arm64", 2}, nil }
func (f *fakeRuntime) Logs(context.Context, string, io.Writer) error        { return nil }
func (f *fakeRuntime) CopyIn(context.Context, string, string, string) error { return nil }
func (f *fakeRuntime) RemoveLabeled() (int, error)                          { return 0, nil }

func (f *fakeRuntime) Build(context.Context, string, string, bool, io.Writer) error {
	f.builds++
	return f.buildErr
}

func (f *fakeRuntime) Start(_ context.Context, name, _ string, _ *InitSystem) error {
	f.started = append(f.started, name)
	return nil
}

func (f *fakeRuntime) Remove(name string) error {
	f.removed = append(f.removed, name)
	return nil
}

func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string, out io.Writer, argv ...string) (int, error) {
	f.execs = append(f.execs, argv)
	switch {
	case slices.Equal(argv, []string{"systemctl", "is-system-running"}):
		if f.ready {
			fmt.Fprintln(out, "running")
			return 0, nil
		}
		fmt.Fprintln(out, "starting")
		return 1, nil
	case slices.Equal(argv, []string{"systemctl", "--version"}):
		fmt.Fprintln(out, "systemd 252 (252.1)\n+PAM")
	case slices.Contains(argv, "-test.list"):
		fmt.Fprint(out, f.list)
	case slices.Contains(argv, "-test.v"):
		return f.testExit, f.testErr
	}
	return 0, nil
}

func (f *fakeRuntime) ran(argv0 string) bool {
	return slices.ContainsFunc(f.execs, func(a []string) bool { return a[0] == argv0 })
}

func (f *fakeRuntime) ranTests() bool {
	return slices.ContainsFunc(f.execs, func(a []string) bool { return slices.Contains(a, "-test.v") })
}

func testOptions(t *testing.T) options {
	return options{
		timeout:     time.Minute,
		bootTimeout: 30 * time.Millisecond,
		poll:        5 * time.Millisecond,
		cgroups:     2,
		outDir:      t.TempDir(),
		runID:       "1",
		logf:        func(string, ...any) {},
	}
}

func TestRunTarget(t *testing.T) {
	base := Target{Name: "t", Init: systemd, Suites: []string{"allusers"}}
	bestEffort := base
	bestEffort.BestEffort = true
	cgroupV1 := base
	cgroupV1.RequiresCgroupV1 = true

	tests := []struct {
		name       string
		target     Target
		rt         *fakeRuntime
		keep       bool
		want       Status
		noTests    bool
		reason     string
		wantStart  bool
		wantTests  bool
		wantRemove bool
		wantLogs   bool
	}{
		{name: "pass", target: base, rt: &fakeRuntime{ready: true, list: "TestA\n"},
			want: Pass, reason: "systemd 252 (252.1)", wantStart: true, wantTests: true, wantRemove: true},
		{name: "tests fail", target: base, rt: &fakeRuntime{ready: true, list: "TestA\n", testExit: 1},
			want: Fail, reason: "tests failed", wantStart: true, wantTests: true, wantRemove: true, wantLogs: true},
		{name: "runtime fails while testing", target: base, rt: &fakeRuntime{ready: true, list: "TestA\n", testExit: 125, testErr: errors.New("exit 125")},
			want: Fail, reason: "runtime failed", wantStart: true, wantTests: true, wantRemove: true, wantLogs: true},
		{name: "no boot", target: base, rt: &fakeRuntime{},
			want: Fail, reason: "not ready", wantStart: true, wantRemove: true},
		{name: "no boot, best effort", target: bestEffort, rt: &fakeRuntime{},
			want: Skip, reason: "not ready", wantStart: true, wantRemove: true},
		{name: "best effort still fails its tests", target: bestEffort, rt: &fakeRuntime{ready: true, list: "TestA\n", testExit: 1},
			want: Fail, wantStart: true, wantTests: true, wantRemove: true, wantLogs: true},
		{name: "cgroup v1 only, on v2", target: cgroupV1, rt: &fakeRuntime{ready: true, list: "TestA\n"},
			want: Skip, reason: "cgroup v1"},
		{name: "image build fails", target: base, rt: &fakeRuntime{buildErr: errors.New("boom")},
			want: Fail, reason: "image build failed"},
		{name: "no test matches", target: base, rt: &fakeRuntime{ready: true, list: "ok  \n"},
			noTests: true, wantStart: true, wantRemove: true},
		{name: "keep a failed container", target: base, rt: &fakeRuntime{ready: true, list: "TestA\n", testExit: 1}, keep: true,
			want: Fail, reason: "exec -it", wantStart: true, wantTests: true, wantLogs: true},
		{name: "keep removes a passing container", target: base, rt: &fakeRuntime{ready: true, list: "TestA\n"}, keep: true,
			want: Pass, wantStart: true, wantTests: true, wantRemove: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t)
			o.keep = tt.keep
			rt := tt.rt
			rs := runTarget(context.Background(), rt, tt.target, o)
			if len(rs) != 1 {
				t.Fatalf("got %d results, want 1", len(rs))
			}
			r := rs[0]
			if r.NoTests != tt.noTests || (!tt.noTests && r.Status != tt.want) {
				t.Errorf("status %s, noTests %v; want %s, noTests %v (%s)", r.Status, r.NoTests, tt.want, tt.noTests, r.Reason)
			}
			if !strings.Contains(r.Reason, tt.reason) {
				t.Errorf("reason %q, want it to contain %q", r.Reason, tt.reason)
			}
			if got := len(rt.started) == 1; got != tt.wantStart {
				t.Errorf("started %v, want one start: %v", rt.started, tt.wantStart)
			}
			if got := rt.ranTests(); got != tt.wantTests {
				t.Errorf("ran tests %v, want %v", got, tt.wantTests)
			}
			if got := slices.Equal(rt.removed, rt.started) && len(rt.removed) > 0; got != tt.wantRemove {
				t.Errorf("removed %v of %v, want removed: %v", rt.removed, rt.started, tt.wantRemove)
			}
			if got := rt.ran("journalctl"); got != tt.wantLogs {
				t.Errorf("collected init logs %v, want %v", got, tt.wantLogs)
			}
			if tt.wantStart && !rt.ran("touch") && r.Status == Pass {
				t.Error("marker not written")
			}
			if tt.wantTests {
				if _, err := os.Stat(filepath.Join(o.outDir, "t", "allusers.log")); err != nil {
					t.Errorf("no suite log: %v", err)
				}
			}
		})
	}
}

func TestRunSuiteTestArgv(t *testing.T) {
	o := testOptions(t)
	o.runPattern = "^TestX$"
	rt := &fakeRuntime{ready: true, list: "TestX\n"}
	runTarget(context.Background(), rt, Target{Name: "t", Init: systemd, Suites: []string{"allusers"}}, o)
	i := slices.IndexFunc(rt.execs, func(a []string) bool { return slices.Contains(a, "-test.v") })
	if i < 0 {
		t.Fatal("tests not run")
	}
	want := []string{"/opt/test/systemd_container.test", "-test.v", "-test.run", "^TestX$", "-test.timeout", "1m0s"}
	if got := rt.execs[i]; !slices.Equal(got, want) {
		t.Errorf("test argv = %q, want %q", got, want)
	}
}

type envRuntime struct {
	fakeRuntime
	env []string
}

func (e *envRuntime) Exec(ctx context.Context, name string, env []string, out io.Writer, argv ...string) (int, error) {
	if slices.Contains(argv, "-test.v") {
		e.env = env
	}
	return e.fakeRuntime.Exec(ctx, name, env, out, argv...)
}

func TestRunSuiteSetsExpectPlatform(t *testing.T) {
	rt := &envRuntime{fakeRuntime: fakeRuntime{ready: true, list: "TestA\n"}}
	runTarget(context.Background(), rt, Target{Name: "t", Init: systemd, Suites: []string{"allusers"}}, testOptions(t))
	if want := []string{"EXPECT_PLATFORM=linux-systemd"}; !slices.Equal(rt.env, want) {
		t.Errorf("env = %q, want %q", rt.env, want)
	}
}

func TestRunTargetInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rt := &fakeRuntime{}
	rs := runTarget(ctx, rt, Target{Name: "t", Init: systemd, Suites: []string{"allusers"}}, testOptions(t))
	if rs[0].Status != Fail || rs[0].Reason != "interrupted" {
		t.Errorf("got %s %q, want FAIL interrupted", rs[0].Status, rs[0].Reason)
	}
	if !slices.Equal(rt.removed, rt.started) {
		t.Errorf("removed %v of %v", rt.removed, rt.started)
	}
}

func TestCountTests(t *testing.T) {
	if n := countTests("TestA\nTestB\nExampleC\nok  \tpkg\t0.1s\n"); n != 2 {
		t.Errorf("countTests = %d, want 2", n)
	}
}

func TestSummarize(t *testing.T) {
	pass := Result{Target: "a", Suite: "s", Status: Pass}
	fail := Result{Target: "b", Suite: "s", Status: Fail}
	skip := Result{Target: "c", Suite: "s", Status: Skip}
	none := Result{Target: "d", Suite: "s", NoTests: true}
	tests := []struct {
		name string
		rs   []Result
		want int
	}{
		{"all pass", []Result{pass}, 0},
		{"one fails", []Result{pass, fail, skip}, 1},
		{"skips only", []Result{skip}, 0},
		{"pass and no tests", []Result{pass, none}, 0},
		{"no tests anywhere", []Result{none, none}, 1},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		if got := summarize(&buf, tt.rs, "/"); got != tt.want {
			t.Errorf("%s: exit %d, want %d", tt.name, got, tt.want)
		}
		if strings.Contains(buf.String(), "  d  s") {
			t.Errorf("%s: a suite with no matching tests is in the summary:\n%s", tt.name, buf.String())
		}
	}
}

func TestFindRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "linux_test", "harness")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(root, "go.mod"), "module "+rootModule+"\n\ngo 1.23.0\n")
	write(filepath.Join(nested, "go.mod"), "module "+rootModule+"/linux_test/harness\n")
	if got, err := findRoot(nested); err != nil || got != root {
		t.Errorf("findRoot = %q, %v, want %q", got, err, root)
	}
	if _, err := findRoot(t.TempDir()); err == nil {
		t.Error("findRoot outside the repository: no error")
	}
}
