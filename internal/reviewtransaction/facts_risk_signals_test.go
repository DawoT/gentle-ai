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

// Removing frozen regular-resource verification must break the positive cases;
// accepting unchecked resources must break the optional-miss cases.
func TestFactsRiskSignalsJSONResources(t *testing.T) {
	for _, tc := range []struct {
		name, importer, target, specifier, evidence, mode string
		wantSignals, testsOnly                            bool
	}{
		{"real-production-edge", "lib/runtime-metrics-native.ts", "contracts/telemetry/runtime-aggregate-v1.schema.json", "../contracts/telemetry/runtime-aggregate-v1.schema.json", "filesystem", "", true, true},
		// Outgoing imports of unchanged production/resources do not make tests consumers' production inputs.
		{"test-production", "tests/importer.test.ts", "contracts/data.json", "../contracts/data.json", "filesystem", "", true, true},
		{"test-test", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "", true, true},
		{"production-consumer-with-resource", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "incoming", true, false},
		{"production-transitive-consumer-with-resource", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "transitive", true, false},
		{"missing-base", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "missing-base", false, false},
		{"missing-candidate", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "missing-candidate", false, false},
		{"typescript-resource", "tests/importer.test.ts", "tests/data.json", "./data.json", "typescript", "", false, false},
		{"unknown-evidence", "tests/importer.test.ts", "tests/data.json", "./data.json", "unknown", "", false, false},
		{"mismatch", "tests/importer.test.ts", "tests/data.json", "./other.json", "filesystem", "", false, false},
		{"absolute", "tests/importer.test.ts", "tests/data.json", "/tests/data.json", "filesystem", "", false, false},
		{"unsafe-target", "tests/importer.test.ts", "tests/../tests/data.json", "./data.json", "filesystem", "", false, false},
		{"backslash", "tests/importer.test.ts", "tests/data.json", ".\\data.json", "filesystem", "", false, false},
		{"unsupported-extension", "tests/importer.test.ts", "tests/data.txt", "./data.txt", "filesystem", "", false, false},
		{"symlink", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "symlink", false, false},
		{"directory", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "directory", false, false},
		{"submodule", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "submodule", false, false},
		{"missing-blob", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "missing-blob", false, false},
		{"changed-resource", "tests/importer.test.ts", "tests/data.json", "./data.json", "filesystem", "changed", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			git := func(input string, args ...string) string {
				t.Helper()
				cmd := exec.Command("git", append([]string{"--no-replace-objects", "-C", root}, args...)...)
				cmd.Stdin = strings.NewReader(input)
				cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
				b, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v %s", args, err, b)
				}
				return strings.TrimSpace(string(b))
			}
			git("", "init")
			cache := filepath.Join(root, ".pi", "facts-commit-cache")
			artifact := func(kind string, value any) string {
				t.Helper()
				b, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(b)
				id := hex.EncodeToString(h[:])
				dir := filepath.Join(cache, "facts-data", kind)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0600); err != nil {
					t.Fatal(err)
				}
				return id
			}
			changed := "tests/change.test.ts"
			if strings.HasPrefix(tc.importer, "tests/") {
				changed = tc.importer
			}
			var parent, baseTree, candidateTree string
			for depth := 0; depth < 2; depth++ {
				refs := map[string]string{}
				entries := ""
				paths := []string{tc.importer}
				if changed != tc.importer {
					paths = append(paths, changed)
				}
				if tc.mode == "incoming" || tc.mode == "transitive" {
					paths = append(paths, "lib/consumer.ts")
				}
				if tc.mode == "transitive" {
					paths = append(paths, "tests/bridge.test.ts")
				}
				for _, p := range paths {
					content := "export const value = 1;\n"
					if p == changed && depth == 1 {
						content += "// changed\n"
					}
					blob := git(content, "hash-object", "-w", "--stdin")
					entries += fmt.Sprintf("100644 blob %s\t%s\n", blob, p)
					refs[p] = artifact("objects", map[string]any{"path": p, "sha": blob, "symbols": []any{}})
				}
				resource := git("{\"type\":\"object\"}\n", "hash-object", "-w", "--stdin")
				mode, kind := "100644", "blob"
				switch tc.mode {
				case "missing-blob":
					resource = strings.Repeat("a", 40)
				case "symlink":
					mode = "120000"
				case "directory":
					mode, kind, resource = "040000", "tree", git("", "mktree")
				case "submodule":
					mode, kind, resource = "160000", "commit", git("submodule\n", "commit-tree", git("", "mktree"))
				case "changed":
					if depth == 1 {
						resource = git("{}\n", "hash-object", "-w", "--stdin")
					}
				}
				if !(tc.mode == "missing-base" && depth == 0 || tc.mode == "missing-candidate" && depth == 1) {
					// Build nested trees without ever creating resource files in the worktree.
					parts := strings.Split(filepath.ToSlash(filepath.Clean(tc.target)), "/")
					entry := fmt.Sprintf("%s %s %s\t%s\n", mode, kind, resource, parts[len(parts)-1])
					for i := len(parts) - 2; i >= 0; i-- {
						entry = fmt.Sprintf("040000 tree %s\t%s\n", git(entry, "mktree", "--missing"), parts[i])
					}
					// Combine resource and indexed sources using a temporary index owned by this fixture.
					git("", "read-tree", git(entry, "mktree", "--missing"))
				} else {
					git("", "read-tree", git("", "mktree"))
				}
				git(entries, "update-index", "--index-info")
				tree := git("", "write-tree", "--missing-ok")
				commit := git("fixture\n", "commit-tree", tree)
				baseTree, candidateTree = candidateTree, tree
				edges := []map[string]string{{"importer": tc.importer, "target": tc.target, "specifier": tc.specifier, "evidence": tc.evidence}}
				if tc.mode == "incoming" {
					edges = append(edges, map[string]string{"importer": "lib/consumer.ts", "target": changed, "evidence": "typescript"})
				}
				if tc.mode == "transitive" {
					edges = append(edges, map[string]string{"importer": "lib/consumer.ts", "target": "tests/bridge.test.ts", "evidence": "typescript"}, map[string]string{"importer": "tests/bridge.test.ts", "target": changed, "evidence": "typescript"})
				}
				parent = artifact("generations", map[string]any{"format": "facts-generation-v1", "depth": depth, "parent": parent, "metadata": map[string]any{"version": "1.3.0", "source": map[string]string{"kind": "commit", "commit": commit, "scope": "."}, "moduleEdges": edges}, "upserts": refs, "deleted": []string{}})
			}
			b, _ := json.Marshal(map[string]string{"format": "facts-pointer-v1", "generation": parent})
			if err := os.WriteFile(filepath.Join(cache, "facts.json"), b, 0600); err != nil {
				t.Fatal(err)
			}
			paths := []string{changed}
			if tc.mode == "changed" {
				paths = append(paths, tc.target)
			}
			signals, err := ReadFactsRiskSignals(root, baseTree, candidateTree, paths)
			if err != nil {
				t.Fatal(err)
			}
			if (signals != nil) != tc.wantSignals {
				t.Fatalf("signals = %+v; want present %v", signals, tc.wantSignals)
			}
			if signals != nil && (!signals.CoverageComplete || signals.TestsOnly != tc.testsOnly) {
				t.Fatalf("signals = %+v; want testsOnly %v", signals, tc.testsOnly)
			}
			if tc.wantSignals {
				assessment, err := (SnapshotBuilder{Repo: root}).AssessSnapshotRisk(t.Context(), Snapshot{BaseTree: baseTree, CandidateTree: candidateTree, Paths: paths})
				if err != nil {
					t.Fatal(err)
				}
				want := RiskLow
				if !tc.testsOnly {
					want = RiskMedium
				}
				if assessment.Level != want {
					t.Fatalf("tier = %s want %s", assessment.Level, want)
				}
			}
		})
	}
}

// A failed or incomplete Git lookup cannot establish the external boundary.
func TestFactsRiskSignalsDependencyProofFailures(t *testing.T) {
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	for _, tree := range []string{"", "not-a-tree", strings.Repeat("a", 40)} {
		if factsRiskDependencyRootAbsent(root, tree) {
			t.Fatalf("invalid/unavailable tree %q proved absent", tree)
		}
	}
	if factsRiskDependencyRootAbsent(filepath.Join(root, "missing"), strings.Repeat("a", 40)) {
		t.Fatal("unavailable repository proved absent")
	}
	t.Setenv("PATH", "")
	if factsRiskDependencyRootAbsent(root, strings.Repeat("a", 40)) {
		t.Fatal("unavailable Git proved absent")
	}
}

func TestFactsRiskSignalsStaticGrammarAndPythonProofFailures(t *testing.T) {
	for _, spec := range []string{"pkg/../file.js", "pkg/./file.js", "pkg/.../file.js", "./pkg/file.js", "/pkg/file.js", "#alias/file.js", "@/file.js", "https://host/file.js", "pkg\\file.js", "pkg/file.js?raw", "pkg/file.js#fragment", "node:path/../fs"} {
		if factsRiskNonlocalSpecifier(spec) {
			t.Errorf("unsafe specifier accepted: %q", spec)
		}
	}
	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, b)
	}
	for _, tree := range []string{"", "not-a-tree", strings.Repeat("a", 40)} {
		if factsRiskPythonUnshadowed(root, tree) {
			t.Errorf("unavailable tree proved unshadowed: %q", tree)
		}
	}
	if factsRiskPythonUnshadowed(filepath.Join(root, "missing"), strings.Repeat("a", 40)) {
		t.Fatal("missing repo proved unshadowed")
	}
	t.Setenv("PATH", "")
	if factsRiskPythonUnshadowed(root, strings.Repeat("a", 40)) {
		t.Fatal("missing Git proved unshadowed")
	}
}

func TestFactsRiskSignals(t *testing.T) {
	cases := []struct {
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
		{"real-relative-dependency", "tests/shell-sidebar-fullscreen.test.ts", "dependency", 0, 0, RiskLow, "facts_tests_only_change"},
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
	}
	for _, mode := range []string{
		"unscoped", "cross", "cross-out", "sibling", "missing-importer", "missing-evidence", "missing-reason", "filled-target",
		"traversal", "dot-tail", "nested-root", "invalid-package", "invalid-scope", "missing-subpath", "empty-subpath", "absolute", "url", "backslash", "query", "alias", "local-root",
		"root-file-base", "root-file-candidate", "root-symlink-base", "root-symlink-candidate",
		"root-submodule-base", "root-submodule-candidate", "root-empty-base", "root-empty-candidate",
		"root-content-base", "root-content-candidate",
	} {
		want, reason := RiskMedium, ""
		// A changed test's outgoing production dependency is not a consumer.
		if mode == "unscoped" || mode == "cross-out" {
			want, reason = RiskLow, "facts_tests_only_change"
		}
		cases = append(cases, struct {
			name, path, mode  string
			dependents, delta int
			want              RiskLevel
			reason            string
		}{"dependency-" + mode, "tests/shell-sidebar-fullscreen.test.ts", "dependency-" + mode, 0, 0, want, reason})
	}
	for _, tc := range []struct {
		mode, path string
		positive   bool
	}{
		{"dotted", "a_test.go", true},
		{"stdlib-go", "a_test.go", true}, {"stdlib-python", "tests/a.py", true},
		{"stdlib-sys", "tests/a.py", true},
		{"stdlib-unknown", "tests/a.py", false}, {"stdlib-relative", "tests/a.py", false},
		{"stdlib-unknown-go", "a_test.go", false}, {"stdlib-python-go", "a_test.go", false},
		{"stdlib-missing-importer", "tests/a.py", false}, {"stdlib-wrong-evidence", "tests/a.py", false},
		{"stdlib-wrong-language", "tests/a.ts", false}, {"stdlib-go-python", "tests/a.py", false},
		{"stdlib-wrong-reason", "tests/a.py", false}, {"stdlib-filled", "tests/a.py", false},
		{"stdlib-shadow-file-base", "tests/a.py", false}, {"stdlib-shadow-file-candidate", "tests/a.py", false},
		{"stdlib-shadow-package-base", "tests/a.py", false}, {"stdlib-shadow-package-candidate", "tests/a.py", false},
		{"stdlib-shadow-pyc-base", "tests/a.py", false}, {"stdlib-shadow-pyc-candidate", "tests/a.py", false},
		{"stdlib-shadow-compiled-base", "tests/a.py", false}, {"stdlib-shadow-compiled-candidate", "tests/a.py", false},
		{"stdlib-shadow-archive-base", "tests/a.py", false}, {"stdlib-shadow-archive-candidate", "tests/a.py", false},
		{"stdlib-shadow-symlink-base", "tests/a.py", false}, {"stdlib-shadow-symlink-candidate", "tests/a.py", false},
		{"stdlib-shadow-gitlink-base", "tests/a.py", false}, {"stdlib-shadow-gitlink-candidate", "tests/a.py", false},
	} {
		want, reason := RiskMedium, ""
		if tc.positive {
			want, reason = RiskLow, "facts_tests_only_change"
		}
		cases = append(cases, struct {
			name, path, mode  string
			dependents, delta int
			want              RiskLevel
			reason            string
		}{tc.mode, tc.path, tc.mode, 0, 0, want, reason})
	}
	for _, tc := range []struct {
		mode string
		low  bool
	}{
		{"outgoing", true}, {"incoming", false}, {"transitive", false},
		{"cycle-test", true}, {"cycle-production", false},
		{"base-only", false}, {"candidate-only", false},
		{"deleted-test", false}, {"deleted-consumer", false}, {"changed-production", false},
		{"incoming-resource", false}, {"transitive-resource", false},
	} {
		want, reason := RiskMedium, ""
		if tc.low {
			want, reason = RiskLow, "facts_tests_only_change"
		}
		p := "tests/change.test.ts"
		if strings.HasSuffix(tc.mode, "-resource") {
			p = "tests/data.json"
		}
		cases = append(cases, struct {
			name, path, mode  string
			dependents, delta int
			want              RiskLevel
			reason            string
		}{"graph-" + tc.mode, p, "graph-" + tc.mode, 0, 0, want, reason})
	}
	for _, tc := range cases {
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
				if tc.mode == "mixed" || tc.mode == "cross-node" || tc.mode == "dependency-cross" || tc.mode == "dependency-cross-out" || tc.mode == "dependency-sibling" {
					paths = append(paths, "dep.go")
				}
				if strings.HasPrefix(tc.mode, "graph-") {
					paths = append(paths, "tests/bridge.test.ts", "lib/consumer.ts")
					if tc.mode == "graph-deleted-test" && depth == 1 {
						paths = paths[1:]
					}
					if tc.mode == "graph-deleted-consumer" && depth == 1 {
						paths = []string{tc.path, "tests/bridge.test.ts"}
					}
				}
				for i, p := range paths {
					content := "package example\n"
					if depth == 1 && (p == tc.path || tc.mode == "graph-changed-production" && p == "lib/consumer.ts") {
						content += "// changed\n"
					}
					blob := git(content, "hash-object", "-w", "--stdin")
					entries += fmt.Sprintf("100644 blob %s\t%s\n", blob, p)
					symbols := []map[string]any{}
					if depth == 1 && i == 0 && tc.delta > 0 {
						symbols = append(symbols, map[string]any{"name": "New", "kind": "function", "signature": "New()", "isExported": true})
					}
					refs[p] = artifact("objects", map[string]any{"path": p, "sha": blob, "symbols": symbols})
					if i > 0 && !strings.HasPrefix(tc.mode, "graph-") {
						evidence := "filesystem"
						if tc.mode == "resolved-typescript" {
							evidence = "typescript"
						} else if tc.mode == "resolved-unknown" {
							evidence = "unknown"
						}
						edge := map[string]string{"importer": p, "specifier": "./" + tc.path, "target": tc.path, "evidence": evidence}
						if tc.mode == "dependency-cross-out" {
							edge = map[string]string{"importer": tc.path, "specifier": "../dep.go", "target": p, "evidence": "filesystem"}
						}
						if tc.mode == "dependency-sibling" {
							edge = map[string]string{"importer": p, "specifier": "./missing", "evidence": "unresolved", "reason": "module-not-found"}
						}
						edges = append(edges, edge)
					}
				}
				if tc.mode == "unresolved" {
					edges = append(edges, map[string]string{"importer": tc.path, "target": "", "evidence": "unresolved"})
				}
				if strings.HasPrefix(tc.mode, "graph-") {
					add := func(importer, target string) {
						edges = append(edges, map[string]string{"importer": importer, "target": target, "evidence": "typescript"})
					}
					active := !(tc.mode == "graph-base-only" && depth == 1 || tc.mode == "graph-candidate-only" && depth == 0 || (tc.mode == "graph-deleted-test" || tc.mode == "graph-deleted-consumer") && depth == 1)
					if active {
						switch tc.mode {
						case "graph-outgoing":
							add(tc.path, "lib/consumer.ts")
						case "graph-incoming", "graph-incoming-resource", "graph-deleted-consumer":
							add("lib/consumer.ts", tc.path)
						default:
							add("tests/bridge.test.ts", tc.path)
							if tc.mode == "graph-cycle-test" || tc.mode == "graph-cycle-production" {
								add(tc.path, "tests/bridge.test.ts")
							}
							if tc.mode != "graph-cycle-test" {
								add("lib/consumer.ts", "tests/bridge.test.ts")
							}
						}
					}
				}
				// These edges mirror Pi's unresolved schema, including reason/specifier.
				if specifier, ok := map[string]string{
					"node": "node:path", "external": "lodash", "scoped": "@scope/pkg/subpath", "dotted": "@earendil-works/pi-tui/dist/layout.js",
					"relative": "./helper", "absolute": "/src/helper", "empty-specifier": "",
					"missing-specifier": "lodash", "unknown-reason": "lodash", "empty-reason": "lodash",
					"unexpected-target": "lodash", "missing-importer": "lodash", "unknown-importer": "lodash",
					"unsupported": "lodash", "unindexed-target": "./data.json", "alias": "#helpers",
					"path-alias": "@/helpers", "parent-segment": "pkg/../helper", "malformed-node": "node:",
					"cross-node": "node:path",
					"dependency": "../node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/chat-viewport.js",
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
				if strings.HasPrefix(tc.mode, "stdlib-") {
					specs := []string{"ast", "copy", "json"}
					switch tc.mode {
					case "stdlib-go":
						specs = []string{"bytes", "encoding/json", "fmt", "go/ast", "go/parser", "go/printer", "go/token", "os", "strconv", "strings"}
					case "stdlib-sys":
						specs = []string{"sys"}
					case "stdlib-unknown", "stdlib-unknown-go":
						specs = []string{"custom"}
					case "stdlib-relative":
						specs = []string{"./ast"}
					case "stdlib-go-python":
						specs = []string{"bytes"}
					}
					for _, spec := range specs {
						edge := map[string]string{"importer": tc.path, "specifier": spec, "evidence": "unresolved", "reason": "language-resolution-not-supported"}
						if tc.mode == "stdlib-wrong-reason" {
							edge["reason"] = "unknown-resolution"
						}
						if tc.mode == "stdlib-filled" {
							edge["target"] = tc.path
						}
						if tc.mode == "stdlib-missing-importer" {
							edge["importer"] = "missing.py"
						}
						if tc.mode == "stdlib-wrong-evidence" {
							edge["evidence"] = "unsupported"
						}
						edges = append(edges, edge)
					}
				}
				if strings.HasPrefix(tc.mode, "dependency-") {
					specifier := "../node_modules/@earendil-works/pi-coding-agent/dist/modes/interactive/chat-viewport.js"
					if s, ok := map[string]string{
						"unscoped":        "../node_modules/pkg.name/dist/file.test.js",
						"traversal":       "../node_modules/pkg/../other/file.js",
						"dot-tail":        "../node_modules/pkg/./file.js",
						"nested-root":     "../node_modules/pkg/node_modules/other/file.js",
						"invalid-package": "../node_modules/.pkg/file.js",
						"invalid-scope":   "../node_modules/@/pkg/file.js",
						"missing-subpath": "../node_modules/pkg",
						"empty-subpath":   "../node_modules/pkg/",
						"absolute":        "/node_modules/pkg/file.js", "url": "https://host/node_modules/pkg/file.js",
						"backslash": "../node_modules/pkg\\file.js", "query": "../node_modules/pkg/file.js?raw",
						"alias": "@/node_modules/pkg/file.js", "local-root": "./node_modules/pkg/file.js",
					}[strings.TrimPrefix(tc.mode, "dependency-")]; ok {
						specifier = s
					}
					edge := map[string]string{"importer": tc.path, "specifier": specifier, "evidence": "unresolved", "reason": "module-not-found"}
					switch tc.mode {
					case "dependency-missing-importer":
						delete(edge, "importer")
					case "dependency-missing-evidence":
						delete(edge, "evidence")
					case "dependency-missing-reason":
						delete(edge, "reason")
					case "dependency-filled-target":
						edge["target"] = tc.path
					}
					edges = append(edges, edge, edge) // Repeated edges share one proof per tree.
				}
				if tc.mode == "missing-path" {
					refs, entries = map[string]string{}, ""
				}
				if tc.mode == "nil-edges" {
					edges = nil
				}
				rootEntry := ""
				if strings.HasPrefix(tc.mode, "dependency-root-") && (strings.HasSuffix(tc.mode, "-base") && depth == 0 || strings.HasSuffix(tc.mode, "-candidate") && depth == 1) {
					mode, kind, oid := "100644", "blob", git("tracked\n", "hash-object", "-w", "--stdin")
					switch strings.Split(tc.mode, "-")[2] {
					case "symlink":
						mode = "120000"
					case "submodule":
						mode, kind, oid = "160000", "commit", git("submodule\n", "commit-tree", git("", "mktree"))
					case "empty":
						mode, kind, oid = "040000", "tree", git("", "mktree")
					case "content":
						mode, kind, oid = "040000", "tree", git(fmt.Sprintf("100644 blob %s\tfile.js\n", oid), "mktree")
					}
					rootEntry = fmt.Sprintf("%s %s %s\tnode_modules\n", mode, kind, oid)
				}
				git("", "read-tree", git(rootEntry, "mktree"))
				git(entries, "update-index", "--index-info")
				if strings.HasPrefix(tc.mode, "stdlib-shadow-") && (strings.HasSuffix(tc.mode, "-base") && depth == 0 || strings.HasSuffix(tc.mode, "-candidate") && depth == 1) {
					p, mode, oid := "nested/ast.py", "100644", git("shadow\n", "hash-object", "-w", "--stdin")
					switch strings.Split(tc.mode, "-")[2] {
					case "package":
						p = "nested/ast/__init__.py"
					case "pyc":
						p = "nested/__pycache__/ast.cpython-313.pyc"
					case "compiled":
						p = "nested/ast.cpython-313.so"
					case "archive":
						p = "nested/vendor.zip"
					case "symlink":
						p, mode = "unrelated-link", "120000"
					case "gitlink":
						p, mode, oid = "vendor", "160000", git("submodule\n", "commit-tree", git("", "mktree"))
					}
					git(fmt.Sprintf("%s %s\t%s\n", mode, oid, p), "update-index", "--index-info")
				}
				tree := git("", "write-tree")
				if rootEntry != "" && strings.Contains(tc.mode, "-empty-") {
					// An index drops empty directories; construct this root literally.
					tree = git(git("", "ls-tree", tree)+"\n"+rootEntry, "mktree")
				}
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
				parent = artifact("generations", map[string]any{"format": "facts-generation-v1", "depth": generationDepth, "parent": parent, "metadata": metadata, "upserts": refs, "deleted": func() []string {
					if depth == 1 && tc.mode == "graph-deleted-test" {
						return []string{tc.path}
					}
					if depth == 1 && tc.mode == "graph-deleted-consumer" {
						return []string{"lib/consumer.ts"}
					}
					return []string{}
				}()})
			}
			snapshot := Snapshot{BaseTree: baseTree, CandidateTree: candidateTree, Paths: []string{tc.path}}
			if tc.mode == "graph-deleted-consumer" || tc.mode == "graph-changed-production" {
				snapshot.Paths = append(snapshot.Paths, "lib/consumer.ts")
			}
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
			if strings.HasPrefix(tc.mode, "dependency-root-") || strings.HasPrefix(tc.mode, "stdlib-shadow-") {
				pointer, _ := json.Marshal(map[string]string{"format": "facts-pointer-v1", "generation": parent})
				check(os.WriteFile(filepath.Join(cache, "facts.json"), pointer, 0600))
				signals, err := ReadFactsRiskSignals(root, baseTree, candidateTree, snapshot.Paths)
				check(err)
				if signals != nil {
					t.Fatalf("ambiguous frozen proof returned signals: %+v", signals)
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
			signals, e := ReadFactsRiskSignals(root, baseTree, candidateTree, snapshot.Paths)
			check(e)
			if tc.reason == "" && tc.mode != "" && tc.mode != "mixed" && tc.mode != "cross-node" && tc.mode != "dependency-cross" && tc.mode != "dependency-cross-out" && !strings.HasPrefix(tc.mode, "graph-") && signals != nil {
				t.Fatalf("incomplete evidence returned signals: %+v", signals)
			}
			if tc.reason != "" && (signals == nil || !signals.CoverageComplete || signals.UnchangedDependents != tc.dependents || signals.SymbolSurfaceDelta != tc.delta) {
				t.Fatalf("signals = %+v", signals)
			}
		})
	}
}
