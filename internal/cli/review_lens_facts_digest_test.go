package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// Hand-built Pi artifacts: removing emission or checksum validation breaks these cases.
func TestLensContextFactsDigest(t *testing.T) {
	repo := t.TempDir()
	runReviewCLIGit(t, repo, "init")
	runReviewCLIGit(t, repo, "config", "user.email", "test@example.invalid")
	runReviewCLIGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "a.go")
	runReviewCLIGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	old := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD:a.go"))
	if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("package a\nfunc New() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	runReviewCLIGit(t, repo, "add", "a.go")
	runReviewCLIGit(t, repo, "commit", "-m", "candidate")
	commit := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD^{tree}"))
	baseTree := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", base+"^{tree}"))
	blob := strings.TrimSpace(runReviewCLIGit(t, repo, "rev-parse", "HEAD:a.go"))
	cache := filepath.Join(repo, ".pi", "facts-commit-cache")
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
	file := func(sha, name string) map[string]any {
		return map[string]any{"path": "a.go", "sha": sha, "imports": []string{}, "exports": []string{name}, "symbols": []any{map[string]any{"name": name, "kind": "function", "signature": name + "()", "isExported": true}}}
	}
	generation := func(commit string, depth int, parent, object string) string {
		m := map[string]any{"format": "facts-generation-v1", "depth": depth, "metadata": map[string]any{"version": "1.3.0", "root": repo, "source": map[string]any{"kind": "commit", "commit": commit, "scope": "."}, "moduleEdges": []any{map[string]any{"importer": "b.go", "target": "a.go", "evidence": "filesystem"}}}, "upserts": map[string]string{"a.go": object}, "deleted": []string{}}
		if parent != "" {
			m["parent"] = parent
		}
		return artifact("generations", m)
	}
	parent := generation(base, 0, "", artifact("objects", file(old, "Old")))
	object := artifact("objects", file(blob, "New"))
	id := generation(commit, 1, parent, object)
	pointer, _ := json.Marshal(map[string]string{"format": "facts-pointer-v1", "generation": id})
	builder := reviewtransaction.SnapshotBuilder{Repo: repo}
	snapshot, err := builder.Build(t.Context(), reviewtransaction.Target{Kind: reviewtransaction.TargetExactRevision, BaseRef: base, Revision: commit})
	if err != nil {
		t.Fatal(err)
	}
	inspector, err := builder.PrepareCandidateInspector(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer inspector.Close()
	frozen := inspector.FrozenCandidateContext()
	patch := "diff --git a/a.go b/a.go\n+func New() {}"
	build := func(root string) ([]byte, error) {
		deps := reviewLensContextDeps{inspect: func(_ context.Context, _ reviewLensCandidateInspector, op string, _ int, _ string) ([]byte, error) {
			switch op {
			case "name-status":
				return []byte("M\ta.go\n"), nil
			case "numstat":
				return []byte("1\t0\ta.go\n"), nil
			default:
				return []byte(patch), nil
			}
		}}
		binding := reviewLensContextBinding{Lens: "review-reliability", SubjectHash: "sha256:subject"}
		return reviewLensContextBlock(t.Context(), deps, lensContextBudgetInspector{frozen: frozen}, binding, reviewtransaction.ArtifactSubject{}, frozen, "", root)
	}
	if err := os.WriteFile(filepath.Join(cache, "facts.json"), pointer, 0600); err != nil {
		t.Fatal(err)
	}
	golden, err := build("")
	if err != nil {
		t.Fatal(err)
	}
	emitted, err := build(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(emitted), "Facts subject digest") || bytes.Index(emitted, []byte(reviewLensFactsHeader)) > bytes.Index(emitted, []byte(reviewLensContextPatch+" 0")) {
		t.Fatal("digest missing or after patch")
	}
	for _, block := range [][]byte{golden, emitted} {
		if !bytes.Contains(block, []byte("patches remain primary evidence")) {
			t.Fatal("missing scope sentence")
		}
	}
	// Fill the mandatory prompt exactly: the digest is omitted, never a refusal.
	patch += strings.Repeat("x", reviewLensContextRuntimeBudget("")-len(golden))
	full, err := build("")
	if err != nil {
		t.Fatal(err)
	}
	omitted, err := build(repo)
	if err != nil || !bytes.Equal(full, omitted) {
		t.Fatalf("optional section displaced mandatory evidence: %v", err)
	}
	patch += "x"
	if _, err := build(repo); err == nil {
		t.Fatal("mandatory over-budget block admitted")
	}
	for _, tc := range []struct {
		name    string
		pointer []byte
		tree    string
		corrupt bool
		want    string
	}{
		{"matching", pointer, tree, false, "Facts subject digest\n\"a.go\" modified exports +[\"New\"] -[\"Old\"] changed=[] cross-boundary-edges=1 unchanged-dependents=1\n"},
		{"absent", nil, tree, false, ""},
		{"mismatch", pointer, baseTree, false, ""},
		{"unresolvable-current-changes", pointer, strings.Repeat("0", 40), false, ""},
		{"checksum", pointer, tree, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(cache, "facts.json"), tc.pointer, 0600); err != nil {
				t.Fatal(err)
			}
			if tc.corrupt {
				if err := os.WriteFile(filepath.Join(cache, "facts-data", "objects", object+".json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			f := frozen
			f.CandidateTree = tc.tree
			if got := reviewLensFactsDigest(t.Context(), repo, f); got != tc.want {
				t.Fatalf("digest = %q; want %q", got, tc.want)
			}
			if tc.name == "absent" {
				patch = "diff --git a/a.go b/a.go\n+func New() {}"
				got, err := build(repo)
				if err != nil || !bytes.Equal(got, golden) {
					t.Fatalf("absent cache changed mandatory bytes: %v", err)
				}
			}
		})
	}
}
