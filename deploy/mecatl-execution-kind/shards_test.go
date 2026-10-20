package executionkind_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
