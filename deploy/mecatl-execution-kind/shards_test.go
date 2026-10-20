package executionkind_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

type shardRow struct{ shard, test string }

func readShards(t *testing.T) []shardRow {
	t.Helper()
	data, err := os.ReadFile("shards.txt")
	if err != nil {
		t.Fatal(err)
	}
	var rows []shardRow
	for i, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) > 3 {
			t.Fatalf("shards.txt:%d: want <shard> <test> [<seconds>]", i+1)
		}
		if n, err := strconv.Atoi(fields[0]); fields[0] != "none" && (err != nil || n < 1) {
			t.Fatalf("shards.txt:%d: shard must be a positive integer or none", i+1)
		}
		if len(fields) < 2 {
			t.Fatalf("shards.txt:%d: missing test name", i+1)
		}
		rows = append(rows, shardRow{shard: fields[0], test: fields[1]})
	}
	return rows
}

// clusterTests returns every top-level TestKindExecution* function in the Kind
// suite, regardless of build tags.
func clusterTests(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("../../e2e/k8s_execution/*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no Kind execution test files: %v", err)
	}
	tests := make(map[string]bool)
	fset := token.NewFileSet()
	for _, file := range files {
		parsed, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "TestKindExecution") {
				tests[fn.Name.Name] = true
			}
		}
	}
	return tests
}

// A cluster test that matches no shard would silently stop running in CI; one
// in two shards would run twice. Shard ids must match the workflow matrix.
func TestQualificationShardsPartitionClusterTests(t *testing.T) {
	declared := clusterTests(t)
	owner := make(map[string]string)
	shards := make(map[string]bool)
	for _, row := range readShards(t) {
		if prev, dup := owner[row.test]; dup {
			t.Errorf("%s is listed in shard %s and shard %s", row.test, prev, row.shard)
		}
		owner[row.test] = row.shard
		if !declared[row.test] {
			t.Errorf("shards.txt lists %s, which is not a TestKindExecution* function in e2e/k8s_execution", row.test)
		}
		if row.shard != "none" {
			shards[row.shard] = true
		}
	}
	for name := range declared {
		if _, ok := owner[name]; !ok {
			t.Errorf("%s matches no shard; add it to deploy/mecatl-execution-kind/shards.txt", name)
		}
	}

	data, err := os.ReadFile("../../.github/workflows/k8s-e2e.yml")
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Shard []int `yaml:"shard"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var matrix []string
	for _, shard := range workflow.Jobs["execution-production-e2e"].Strategy.Matrix.Shard {
		matrix = append(matrix, strconv.Itoa(shard))
	}
	var listed []string
	for shard := range shards {
		listed = append(listed, shard)
	}
	slices.Sort(matrix)
	slices.Sort(listed)
	if !slices.Equal(matrix, listed) {
		t.Errorf("workflow matrix shards %v differ from shards.txt shards %v", matrix, listed)
	}
}

func TestQualificationShardSkipsOnlyOtherShards(t *testing.T) {
	serial := scriptRange(t, "run.sh", "set --\nif [ \"${handoff:-0}\"", "\nphase_done tests")
	shards, err := os.ReadFile("shards.txt")
	if err != nil {
		t.Fatal(err)
	}
	rows := readShards(t)
	for _, shard := range []string{"1", "2", "3"} {
		t.Run("shard="+shard, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"state", "e2e/k8s_execution", "deploy/mecatl-execution-kind"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeFixture(t, filepath.Join(root, "deploy/mecatl-execution-kind/shards.txt"), string(shards), 0o600)
			writeFixture(t, filepath.Join(root, "state/qualification.test"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$HOME/args\"\n", 0o700)
			out, err := runStep(t, root, "set -eu\ndev() { (cd \"$root\" && \"$@\"); }\n"+serial, "root="+root, "state="+filepath.Join(root, "state"), "shard="+shard, "kubeconfig=owned", "context=owned", "go_revision=owned", "MECATL_EXECUTION_QUAL_PROFILE=production")
			if err != nil {
				t.Fatalf("shard selection: %v: %s", err, out)
			}
			data, err := os.ReadFile(filepath.Join(root, "args"))
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(args) < 2 || args[0] != "-test.skip" {
				t.Fatalf("shard did not pass a skip selection: %q", args)
			}
			skip := regexp.MustCompile(args[1])
			for _, row := range rows {
				if skip.MatchString(row.test) == (row.shard == shard) {
					t.Errorf("shard %s: %s (shard %s) skipped=%v", shard, row.test, row.shard, skip.MatchString(row.test))
				}
			}
			// Unlisted tests, including a new cluster test, run rather than drop out.
			for _, unlisted := range []string{"TestQualificationSemantics", "TestKindExecutionNotYetSharded", "TestKindExecutionQualificationExtra"} {
				if skip.MatchString(unlisted) {
					t.Errorf("shard %s skips unlisted %s", shard, unlisted)
				}
			}
		})
	}
}

func TestQualificationShardPreflight(t *testing.T) {
	preflight := scriptRange(t, "run.sh", "#!/bin/sh", "\nrun_id=")
	shards, err := os.ReadFile("shards.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ shard, handoff, want string }{
		{"", "0", "admitted natural=1 inflight=1"},
		{"1", "0", "admitted natural=0 inflight=0"},
		{"3", "0", "admitted natural=1 inflight=1"},
		{"9", "0", "MECATL_EXECUTION_QUAL_SHARD must name a shard in shards.txt"},
		{"none", "0", "MECATL_EXECUTION_QUAL_SHARD must name a shard in shards.txt"},
		{"$(touch injected)", "0", "MECATL_EXECUTION_QUAL_SHARD must name a shard in shards.txt"},
		{"1", "1", "handoff requires the complete unsharded suite"},
	} {
		t.Run(tc.shard+"/"+tc.handoff, func(t *testing.T) {
			root := t.TempDir()
			scripts := filepath.Join(root, "deploy/mecatl-execution-kind")
			bin := filepath.Join(root, "bin")
			for _, dir := range []string{scripts, bin} {
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			writeFixture(t, filepath.Join(scripts, "shards.txt"), string(shards), 0o600)
			// Stub only the handoff identity checks; the shard check must reject
			// handoff before any handoff state is consulted.
			writeFixture(t, filepath.Join(scripts, "handoff.sh"), "handoff_ci_identity() { :; }\n", 0o600)
			writeFixture(t, filepath.Join(scripts, "run.sh"), preflight+"\nprintf 'admitted natural=%s inflight=%s\\n' \"$natural_expiry\" \"$inflight_renewal\"\n", 0o700)
			writeFixture(t, filepath.Join(bin, "docker"), "#!/bin/sh\nexit 99\n", 0o700)
			env := []string{"PATH=" + bin + ":" + os.Getenv("PATH"), "CONTAINER_ENGINE=docker", "MECATL_EXECUTION_QUAL_PROFILE=production", "MECATL_EXECUTION_QUAL_NATURAL_EXPIRY=1", "MECATL_EXECUTION_QUAL_SHARD=" + tc.shard, "MECATL_EXECUTION_QUAL_HANDOFF=" + tc.handoff}
			if tc.handoff == "1" {
				output := filepath.Join(root, "output")
				env = append(env, "MECATL_EXECUTION_QUAL_CI=0", "GITHUB_OUTPUT="+output, "MECATL_EXECUTION_QUAL_OUTPUT="+output)
			}
			out, _ := runStep(t, root, "sh ./deploy/mecatl-execution-kind/run.sh", env...)
			if strings.TrimSpace(string(out)) != tc.want {
				t.Fatalf("shard preflight: got %q, want %q", out, tc.want)
			}
			if _, err := os.Stat(filepath.Join(root, "injected")); !os.IsNotExist(err) {
				t.Fatal("shard selector executed as shell code")
			}
		})
	}
}
