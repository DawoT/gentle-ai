package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// GO-S3 premise gate: committed history without --base-ref is not an
// ancestor-based range. U4 dissolves for this fork; keep boundary defaults
// unchanged. Selecting HEAD~1 implicitly would make this test fail.
func TestReviewFacadeStartCommittedOnlyWithoutBaseUsesHEAD(t *testing.T) {
	reviewEnabledHome(t)
	repo := initReviewCLIRepo(t)
	for _, content := range []string{"first committed change\n", "second committed change\n"} {
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		runReviewCLIGit(t, repo, "add", "tracked.txt")
		runReviewCLIGit(t, repo, "commit", "-qm", "committed change")
	}
	for _, flags := range [][]string{nil, {"--committed-only"}} {
		var output bytes.Buffer
		args := append([]string{"--cwd", repo, "--lineage", "premise-gate"}, flags...)
		err := runReviewFacadeStart(context.Background(), args, &output)
		if err == nil || !strings.Contains(err.Error(), reviewStartEmptyCandidateHint) {
			t.Fatalf("start %v = %v, output %s; want empty HEAD-based candidate refusal", flags, err, output.String())
		}
	}
	stores, err := reviewtransaction.DiscoverCompactStores(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stores) != 0 {
		t.Fatalf("empty committed-only start created authority: %#v", stores)
	}
}
