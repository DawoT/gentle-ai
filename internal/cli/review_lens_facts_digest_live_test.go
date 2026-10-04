package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

// Opt-in unbound real-data composer component, not native admitted reviewer prompt.
// Existing producer artifacts are read only; no authority or subject is invented.
func TestLiveFactsDigestCharacterization(t *testing.T) {
	root := os.Getenv("GENTLE_FACTS_LIVE_ROOT")
	if root == "" {
		t.Skip("external real-data fixture absent: set GENTLE_FACTS_LIVE_ROOT")
	}
	control := os.Getenv("GENTLE_FACTS_LIVE_CONTROL_ROOT")
	if control == "" {
		t.Fatal("positive fixture requires GENTLE_FACTS_LIVE_CONTROL_ROOT for missing-cache control")
	}
	ref := func(env, fallback string) string {
		if value := os.Getenv(env); value != "" {
			return value
		}
		return fallback
	}
	target := reviewtransaction.Target{Kind: reviewtransaction.TargetExactRevision,
		BaseRef:  ref("GENTLE_FACTS_LIVE_BASE", "d11971cf5d5c8b2080a362a4d3b6b71b092b71db"),
		Revision: ref("GENTLE_FACTS_LIVE_CANDIDATE", "07cefa0b5669a70d323fc3ae3005832fe54bfd0e")}
	ctx, cancel := context.WithTimeout(t.Context(), reviewLensContextTimeout)
	defer cancel()
	t.Log("unbound real-data composer component, not native admitted reviewer prompt")
	var positiveFrozen reviewtransaction.FrozenCandidateContext
	var positiveWithoutFacts []byte
	for _, fixture := range []struct {
		name, root string
		miss       bool
	}{{"positive", root, false}, {"control", control, true}} {
		t.Run(fixture.name, func(t *testing.T) {
			builder := reviewtransaction.SnapshotBuilder{Repo: fixture.root}
			snapshot, err := builder.Build(ctx, target)
			if err != nil {
				t.Fatal(err)
			}
			inspector, err := builder.PrepareCandidateInspector(ctx, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := inspector.Close(); err != nil {
					t.Error(err)
				}
			}()
			frozen := inspector.FrozenCandidateContext()
			if len(frozen.ChangedPathManifest) == 0 {
				t.Fatal("real candidate has no changed paths")
			}
			digest := reviewLensFactsDigest(ctx, fixture.root, frozen)
			t.Logf("ACTUAL helper base=%s candidate=%s manifest=%+v digest=%q", frozen.BaseTree, frozen.CandidateTree, frozen.ChangedPathManifest, digest)
			if fixture.miss {
				if digest != "" {
					t.Fatalf("missing-cache control returned digest: %q", digest)
				}
				if frozen.BaseTree != positiveFrozen.BaseTree || frozen.CandidateTree != positiveFrozen.CandidateTree {
					t.Fatal("control trees differ")
				}
			} else {
				if digest == "" {
					t.Fatal("real producer cache returned empty digest")
				}
				for _, entry := range frozen.ChangedPathManifest {
					if !strings.Contains(digest, strconv.Quote(entry.Path)) {
						t.Fatalf("digest omits actual changed path %s", entry.Path)
					}
				}
				positiveFrozen = frozen
			}
			// Only the canonical lens configures the renderer. Authority fields and subject stay empty.
			block, err := reviewLensContextBlock(ctx, reviewLensContextDependencies(), inspector,
				reviewLensContextBinding{Lens: "review-reliability"}, reviewtransaction.ArtifactSubject{}, frozen, "", fixture.root)
			if err != nil {
				t.Fatalf("actual unbound composer not verified (helper result above): %v", err)
			}
			section := []byte(reviewLensFactsHeader + "\n" + strings.TrimSpace(digest) + "\n" + reviewLensFactsHeader + "_END\n")
			stripped := block
			if fixture.miss {
				if bytes.Contains(block, []byte(reviewLensFactsHeader+"\n")) {
					t.Fatal("control emitted Facts section")
				}
			} else {
				if bytes.Count(block, section) != 1 {
					t.Fatal("composer did not emit exact helper digest section once")
				}
				stripped = bytes.Replace(block, section, nil, 1)
			}
			patches := 0
			for index, entry := range frozen.ChangedPathManifest {
				if entry.Generated {
					continue
				}
				patch, err := inspector.Inspect(ctx, "patch", index, "")
				if err != nil {
					t.Fatal(err)
				}
				rendered := []byte(fmt.Sprintf("%s %d %s\n%s\n%s_END\n", reviewLensContextPatch, index, entry.Path, bytes.TrimSpace(patch), reviewLensContextPatch))
				at := bytes.Index(block, rendered)
				if at < 0 || len(bytes.TrimSpace(patch)) == 0 {
					t.Fatalf("ordinary immutable patch missing for %s", entry.Path)
				}
				if !fixture.miss && bytes.Index(block, section) >= at {
					t.Fatalf("Facts section not before patch %s", entry.Path)
				}
				patches++
			}
			if patches == 0 {
				t.Fatal("no ordinary immutable patches exercised")
			}
			if fixture.miss {
				if !bytes.Equal(block, positiveWithoutFacts) {
					t.Fatal("control changed ordinary composer bytes beyond Facts omission")
				}
			} else {
				positiveWithoutFacts = stripped
			}
			t.Logf("ACTUAL composer: exact Facts section=%v; all %d ordinary immutable patches retained; Facts precedes every patch=%v; bytes=%d", !fixture.miss, patches, !fixture.miss, len(block))
		})
	}
}
