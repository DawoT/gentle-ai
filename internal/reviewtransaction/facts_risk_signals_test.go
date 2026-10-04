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
		{"resolved-typescript", "a.go", "resolved-typescript", 5, 0, RiskHigh, "facts_unchanged_dependents"},
		{"resolved-filesystem", "a.go", "resolved-filesystem", 5, 0, RiskHigh, "facts_unchanged_dependents"},
		{"resolved-unknown", "a.go", "resolved-unknown", 5, 0, RiskMedium, ""},
		{"surface", "a.go", "", 0, 1, RiskMedium, "facts_symbol_surface_delta"},
		{"zero", "a.go", "", 0, 0, RiskMedium, ""},
		{"absent", "a_test.go", "absent", 0, 0, RiskMedium, ""},
		{"corrupt", "a_test.go", "corrupt", 0, 0, RiskMedium, ""},
		{"wrong-tree", "a_test.go", "wrong", 0, 0, RiskMedium, ""},
		{"mixed", "a_test.go", "mixed", 0, 0, RiskMedium, ""},
		{"unresolved", "a_test.go", "unresolved", 0, 0, RiskMedium, ""},
		{"node-builtin", "a_test.go", "node", 0, 0, RiskLow, "facts_tests_only_change"},
		{"bare-external", "a_test.go", "external", 0, 0, RiskLow, "facts_tests_only_change"},
		{"scoped-external", "a_test.go", "scoped", 0, 0, RiskLow, "facts_tests_only_change"},
		{"relative", "a_test.go", "relative", 0, 0, RiskMedium, ""},
		{"absolute", "a_test.go", "absolute", 0, 0, RiskMedium, ""},
		{"empty-specifier", "a_test.go", "empty-specifier", 0, 0, RiskMedium, ""},
		{"missing-specifier", "a_test.go", "missing-specifier", 0, 0, RiskMedium, ""},
		{"unknown-reason", "a_test.go", "unknown-reason", 0, 0, RiskMedium, ""},
		{"empty-reason", "a_test.go", "empty-reason", 0, 0, RiskMedium, ""},
		{"unexpected-target", "a_test.go", "unexpected-target", 0, 0, RiskMedium, ""},
		{"missing-importer", "a_test.go", "missing-importer", 0, 0, RiskMedium, ""},
		{"unknown-importer", "a_test.go", "unknown-importer", 0, 0, RiskMedium, ""},
		{"unsupported-resolution", "a_test.go", "unsupported", 0, 0, RiskMedium, ""},
		{"unindexed-target", "a_test.go", "unindexed-target", 0, 0, RiskMedium, ""},
		{"nil-edges", "a_test.go", "nil-edges", 0, 0, RiskMedium, ""},
		{"missing-edges", "a_test.go", "missing-edges", 0, 0, RiskMedium, ""},
		{"missing-changed-path", "a_test.go", "missing-path", 0, 0, RiskMedium, ""},
		{"unrelated-base", "a_test.go", "unrelated-base", 0, 0, RiskMedium, ""},
		{"alias", "a_test.go", "alias", 0, 0, RiskMedium, ""},
		{"path-alias", "a_test.go", "path-alias", 0, 0, RiskMedium, ""},
		{"parent-segment", "a_test.go", "parent-segment", 0, 0, RiskMedium, ""},
		{"malformed-node", "a_test.go", "malformed-node", 0, 0, RiskMedium, ""},
		{"production-cross-edge-with-builtin", "a_test.go", "cross-node", 0, 0, RiskMedium, ""},
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
				if tc.mode == "mixed" || tc.mode == "cross-node" {
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
						evidence := "filesystem"
						if tc.mode == "resolved-typescript" {
							evidence = "typescript"
						} else if tc.mode == "resolved-unknown" {
							evidence = "unknown"
						}
						edges = append(edges, map[string]string{"importer": p, "specifier": "./" + tc.path, "target": tc.path, "evidence": evidence})
					}
				}
				if tc.mode == "unresolved" {
					edges = append(edges, map[string]string{"importer": tc.path, "target": "", "evidence": "unresolved"})
				}
				// These edges mirror Pi's unresolved schema, including reason/specifier.
				if specifier, ok := map[string]string{
					"node": "node:path", "external": "lodash", "scoped": "@scope/pkg/subpath",
					"relative": "./helper", "absolute": "/src/helper", "empty-specifier": "",
					"missing-specifier": "lodash", "unknown-reason": "lodash", "empty-reason": "lodash",
					"unexpected-target": "lodash", "missing-importer": "lodash", "unknown-importer": "lodash",
					"unsupported": "lodash", "unindexed-target": "./data.json", "alias": "#helpers",
					"path-alias": "@/helpers", "parent-segment": "pkg/../helper", "malformed-node": "node:",
					"cross-node": "node:path",
				}[tc.mode]; ok {
					edge := map[string]string{"importer": tc.path, "specifier": specifier, "target": "", "evidence": "unresolved", "reason": "module-not-found"}
					switch tc.mode {
					case "missing-specifier":
						delete(edge, "specifier")
					case "unknown-reason":
						edge["reason"] = "unsupported-resolution"
					case "empty-reason":
						edge["reason"] = ""
					case "unexpected-target":
						edge["target"] = tc.path
					case "missing-importer":
						delete(edge, "importer")
					case "unknown-importer":
						edge["importer"] = "missing.go"
					case "unsupported":
						edge["target"], edge["evidence"] = tc.path, "unsupported"
					case "unindexed-target":
						edge["target"], edge["evidence"] = "data.json", "filesystem"
					}
					edges = append(edges, edge)
				}
				if tc.mode == "missing-path" {
					refs, entries = map[string]string{}, ""
				}
				if tc.mode == "nil-edges" {
					edges = nil
				}
				tree := git(entries, "mktree")
				commit := git("fixture\n", "commit-tree", tree)
				baseTree, candidateTree = candidateTree, tree
				source := commit
				if tc.mode == "wrong" && depth == 1 {
					source = strings.Repeat("0", 40)
				}
				metadata := map[string]any{"version": "1.3.0", "source": map[string]string{"kind": "commit", "commit": source, "scope": "."}, "moduleEdges": edges}
				if tc.mode == "missing-edges" {
					delete(metadata, "moduleEdges")
				}
				generationDepth := depth
				if tc.mode == "unrelated-base" {
					generationDepth, parent = 0, ""
				}
				parent = artifact("generations", map[string]any{"format": "facts-generation-v1", "depth": generationDepth, "parent": parent, "metadata": metadata, "upserts": refs, "deleted": []string{}})
			}
			snapshot := Snapshot{BaseTree: baseTree, CandidateTree: candidateTree, Paths: []string{tc.path}}
			builder := SnapshotBuilder{Repo: root}
			if tc.mode == "missing-path" {
				pointer, err := json.Marshal(map[string]string{"format": "facts-pointer-v1", "generation": parent})
				check(err)
				check(os.WriteFile(filepath.Join(cache, "facts.json"), pointer, 0600))
				signals, err := ReadFactsRiskSignals(root, baseTree, candidateTree, snapshot.Paths)
				check(err)
				if signals != nil {
					t.Fatalf("missing changed path returned signals: %+v", signals)
				}
				return
			}
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
			if tc.reason == "" && tc.mode != "" && tc.mode != "mixed" && tc.mode != "cross-node" && signals != nil {
				t.Fatalf("incomplete evidence returned signals: %+v", signals)
			}
			if tc.reason != "" && (signals == nil || !signals.CoverageComplete || signals.UnchangedDependents != tc.dependents || signals.SymbolSurfaceDelta != tc.delta) {
				t.Fatalf("signals = %+v", signals)
			}
		})
	}
}
