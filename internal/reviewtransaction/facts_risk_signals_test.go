package reviewtransaction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFactsRiskSignals(t *testing.T) {
	for _, tc := range []struct {
		name, path, mode  string
		dependents, delta int
		want              RiskLevel
		reason            string
	}{
		{"tests", "a_test.go", "", 0, 0, RiskLow, "facts_tests_only_change"},
		{"process", "a_test.sh", "", 0, 0, RiskHigh, ""},
		{"dependents", "a.go", "", 5, 0, RiskHigh, "facts_unchanged_dependents"},
		{"surface", "a.go", "", 0, 1, RiskMedium, "facts_symbol_surface_delta"},
		{"zero", "a.go", "", 0, 0, RiskMedium, ""},
		{"absent", "a_test.go", "absent", 0, 0, RiskMedium, ""},
		{"corrupt", "a_test.go", "corrupt", 0, 0, RiskMedium, ""},
		{"wrong-tree", "a_test.go", "wrong", 0, 0, RiskMedium, ""},
		{"mixed", "a_test.go", "mixed", 0, 0, RiskMedium, ""},
		{"unresolved", "a_test.go", "unresolved", 0, 0, RiskMedium, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			check := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			git := func(input string, args ...string) string {
				t.Helper()
				c := exec.Command("git", append([]string{"--no-replace-objects", "-C", root}, args...)...)
				c.Stdin = strings.NewReader(input)
				c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
				b, e := c.CombinedOutput()
				if e != nil {
					t.Fatalf("git %v: %v %s", args, e, b)
				}
				return strings.TrimSpace(string(b))
			}
			git("", "init")
			cache := filepath.Join(root, ".pi", "facts-commit-cache")
			artifact := func(kind string, v any) string {
				t.Helper()
				b, e := json.Marshal(v)
				check(e)
				h := sha256.Sum256(b)
				id := hex.EncodeToString(h[:])
				dir := filepath.Join(cache, "facts-data", kind)
				check(os.MkdirAll(dir, 0700))
				check(os.WriteFile(filepath.Join(dir, id+".json"), b, 0600))
				return id
			}
			var parent, baseTree, candidateTree string
			for depth := 0; depth < 2; depth++ {
				refs := map[string]string{}
				entries := ""
				edges := []map[string]string{}
				paths := []string{tc.path}
				for i := 0; i < tc.dependents; i++ {
					paths = append(paths, fmt.Sprintf("dep%d.go", i))
				}
				if tc.mode == "mixed" {
					paths = append(paths, "dep.go")
				}
				for i, p := range paths {
					content := "package example\n"
					if depth == 1 && i == 0 {
						content += "// changed\n"
					}
					blob := git(content, "hash-object", "-w", "--stdin")
					entries += fmt.Sprintf("100644 blob %s\t%s\n", blob, p)
					symbols := []map[string]any{}
					if depth == 1 && i == 0 && tc.delta > 0 {
						symbols = append(symbols, map[string]any{"name": "New", "kind": "function", "signature": "New()", "isExported": true})
					}
					refs[p] = artifact("objects", map[string]any{"path": p, "sha": blob, "symbols": symbols})
					if i > 0 {
						edges = append(edges, map[string]string{"importer": p, "target": tc.path, "evidence": "filesystem"})
					}
				}
				if tc.mode == "unresolved" {
					edges = append(edges, map[string]string{"importer": tc.path, "target": "", "evidence": "unresolved"})
				}
				tree := git(entries, "mktree")
				commit := git("fixture\n", "commit-tree", tree)
				baseTree, candidateTree = candidateTree, tree
				source := commit
				if tc.mode == "wrong" && depth == 1 {
					source = strings.Repeat("0", 40)
				}
				parent = artifact("generations", map[string]any{"format": "facts-generation-v1", "depth": depth, "parent": parent, "metadata": map[string]any{"version": "1.3.0", "source": map[string]string{"kind": "commit", "commit": source, "scope": "."}, "moduleEdges": edges}, "upserts": refs, "deleted": []string{}})
			}
			snapshot := Snapshot{BaseTree: baseTree, CandidateTree: candidateTree, Paths: []string{tc.path}}
			builder := SnapshotBuilder{Repo: root}
			baseline, e := builder.AssessSnapshotRisk(t.Context(), snapshot)
			check(e)
			pointer, _ := json.Marshal(map[string]string{"format": "facts-pointer-v1", "generation": parent})
			if tc.mode == "corrupt" {
				pointer = nil
			}
			if tc.mode != "absent" {
				check(os.WriteFile(filepath.Join(cache, "facts.json"), pointer, 0600))
			}
			got, e := builder.AssessSnapshotRisk(t.Context(), snapshot)
			check(e)
			if got.Level != tc.want {
				t.Fatalf("tier = %s want %s; reasons %v", got.Level, tc.want, got.Reasons)
			}
			found := false
			for _, r := range got.Reasons {
				if string(r.Code) == tc.reason {
					found = true
				}
			}
			if tc.reason != "" && !found {
				t.Fatalf("missing %s: %v", tc.reason, got.Reasons)
			}
			if tc.reason == "" && !reflect.DeepEqual(got, baseline) {
				t.Fatalf("optional miss changed output: %v != %v", got, baseline)
			}
			signals, e := ReadFactsRiskSignals(root, baseTree, candidateTree, []string{tc.path})
			check(e)
			if tc.reason != "" && (signals == nil || !signals.CoverageComplete || signals.UnchangedDependents != tc.dependents || signals.SymbolSurfaceDelta != tc.delta) {
				t.Fatalf("signals = %+v", signals)
			}
		})
	}
}
