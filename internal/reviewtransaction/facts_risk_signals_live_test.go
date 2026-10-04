package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in component evidence against an existing producer cache; never reindexes
// or synthesizes evidence. CI has no external fixture and skips by default.
func TestFactsRiskSignalsLive(t *testing.T) {
	root := os.Getenv("GENTLE_FACTS_LIVE_ROOT")
	if root == "" {
		t.Skip("external real-data fixture absent: set GENTLE_FACTS_LIVE_ROOT")
	}
	runFactsRiskSignalsLive(t, root, false)
}

// The negative fixture is independently opt-in and must remain an optional miss.
func TestFactsRiskSignalsLiveControl(t *testing.T) {
	root := os.Getenv("GENTLE_FACTS_LIVE_CONTROL_ROOT")
	if root == "" {
		t.Skip("external control fixture absent: set GENTLE_FACTS_LIVE_CONTROL_ROOT")
	}
	runFactsRiskSignalsLive(t, root, true)
}

func runFactsRiskSignalsLive(t *testing.T, root string, wantMiss bool) {
	t.Helper()
	resolve := func(env, fallback string) string {
		ref := os.Getenv(env)
		if ref == "" {
			ref = fallback
		}
		b, err := runGit(context.Background(), root, nil, nil, "rev-parse", "--verify", ref+"^{tree}")
		if err != nil {
			t.Fatalf("resolve %s=%s: %v", env, ref, err)
		}
		return strings.TrimSpace(string(b))
	}
	baseTree := resolve("GENTLE_FACTS_LIVE_BASE", "861563e993400a03d9e219dac3f35cea93b143c5")
	candidateTree := resolve("GENTLE_FACTS_LIVE_CANDIDATE", "1aa6f9c1ff4e7a7dd9799fb77c2588df4f423da8")
	// Derive paths from the actual frozen trees, rather than guessing cache paths.
	diff, err := runGit(context.Background(), root, nil, nil, "diff", "--name-only", "-z", baseTree, candidateTree, "--")
	if err != nil {
		t.Fatal(err)
	}
	paths := strings.Split(strings.TrimSuffix(string(diff), "\x00"), "\x00")
	signals, err := ReadFactsRiskSignals(root, baseTree, candidateTree, paths)
	t.Logf("ACTUAL ReadFactsRiskSignals root=%s base=%s candidate=%s paths=%q signals=%+v error=%v", root, baseTree, candidateTree, paths, signals, err)
	diagnoseFactsRiskLive(t, root, baseTree, candidateTree, paths)
	if wantMiss {
		if err != nil || signals != nil {
			t.Fatalf("desired optional miss not met: signals=%+v error=%v", signals, err)
		}
		return
	}
	if err != nil || signals == nil || !signals.TestsOnly || !signals.CoverageComplete {
		t.Fatalf("desired complete facts-only evidence not met: signals=%+v error=%v", signals, err)
	}
}

// These are independent helper checks, not fabricated reader errors/traces.
// Predicate labels refer directly to facts_risk_signals.go.
func diagnoseFactsRiskLive(t *testing.T, root, baseTree, candidateTree string, paths []string) {
	cache := filepath.Join(root, ".pi", "facts-commit-cache")
	budget := int64(32 << 20)
	var pointer struct{ Format, Generation string }
	if !factsRiskRead(filepath.Join(cache, "facts.json"), "", &pointer, &budget) || pointer.Format != "facts-pointer-v1" {
		t.Log("helper check: pointer predicate failed")
		return
	}
	current, ok := factsRiskLoad(cache, pointer.Generation, &budget)
	if !ok || factsRiskTree(root, current) != candidateTree {
		t.Logf("helper check: current load/tree predicate failed load=%v", ok)
		return
	}
	var base factsRiskDB
	for id := current.generation.Parent; id != ""; {
		db, ok := factsRiskLoad(cache, id, &budget)
		if !ok {
			t.Logf("helper check: ancestor load predicate failed id=%s", id)
			return
		}
		if factsRiskTree(root, db) == baseTree {
			base = db
			break
		}
		id = db.generation.Parent
	}
	if base.files == nil {
		t.Log("helper check: base ancestry predicate failed")
		return
	}
	changed := map[string]bool{}
	for _, p := range paths {
		changed[p] = true
		t.Logf("helper check: changed path %s test=%v", p, isTestRiskPath(p))
		if _, b := base.files[p]; !b {
			if _, a := current.files[p]; !a {
				t.Logf("helper check: changed-path presence failed %s", p)
			}
		}
	}
	for _, side := range []struct {
		name, tree string
		db         factsRiskDB
	}{{"base", baseTree, base}, {"candidate", candidateTree, current}} {
		all := append([]string{}, paths...)
		for p := range side.db.files {
			if !changed[p] {
				all = append(all, p)
			}
		}
		blobs, err := treeBlobSizes(context.Background(), root, side.tree, all)
		t.Logf("helper check: %s tree blobs error=%v count=%d indexed=%d", side.name, err, len(blobs), len(side.db.files))
		for _, b := range blobs {
			if side.db.files[b.path].Sha != b.oid {
				t.Logf("helper check: %s SHA mismatch %s", side.name, b.path)
			}
		}
		resources, ok := factsRiskResources(root, side.tree, side.db, changed)
		t.Logf("helper check: %s resources=%v edgesPresent=%v dependencyRootAbsent=%v", side.name, ok, side.db.generation.Metadata.ModuleEdges != nil, factsRiskDependencyRootAbsent(root, side.tree))
		reverse := map[string][]string{}
		outgoing := 0
		for _, e := range side.db.generation.Metadata.ModuleEdges {
			_, importerOK := side.db.files[e.Importer]
			_, targetOK := side.db.files[e.Target]
			targetOK = targetOK || resources[e.Target]
			exempt := e.Evidence == "unresolved" && e.Target == "" && e.Reason == "module-not-found" && (factsRiskNonlocalSpecifier(e.Specifier) || (factsRiskRelativeDependency(e.Importer, e.Specifier) && factsRiskDependencyRootAbsent(root, baseTree) && factsRiskDependencyRootAbsent(root, candidateTree)))
			exempt = exempt || (e.Evidence == "unresolved" && e.Target == "" && e.Reason == "language-resolution-not-supported" && factsRiskKnownStandardImport(e.Importer, e.Specifier, func() bool {
				return factsRiskPythonUnshadowed(root, baseTree) && factsRiskPythonUnshadowed(root, candidateTree)
			}))
			if !importerOK || (!exempt && (!targetOK || (e.Evidence != "filesystem" && e.Evidence != "typescript"))) {
				t.Logf("helper check: %s edge validity failed edge=%+v importerOK=%v targetOK=%v exempt=%v", side.name, e, importerOK, targetOK, exempt)
			}
			if !exempt && importerOK && targetOK && (e.Evidence == "filesystem" || e.Evidence == "typescript") {
				reverse[e.Target] = append(reverse[e.Target], e.Importer)
				if changed[e.Importer] && isTestRiskPath(e.Importer) && !changed[e.Target] && !isTestRiskPath(e.Target) {
					outgoing++
					t.Logf("outgoing test dependency (not a consumer): %s edge=%+v", side.name, e)
				}
			}
		}
		t.Logf("helper check: %s outgoingTestProductionEdges=%d productionReverseConsumer=%v", side.name, outgoing, factsRiskHasProductionConsumer(reverse, changed))
	}
}
