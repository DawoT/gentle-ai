package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gentleman-programming/gentle-ai/v4/internal/reviewtransaction"
)

const reviewLensFactsHeader = "GENTLE_AI_REVIEW_FACTS_SUBJECT_DIGEST"

type lensFactsFile struct {
	Path    string
	Sha     string
	Symbols []struct {
		Name, Kind, Signature string
		IsExported            bool
	}
}
type lensFactsMetadata struct {
	Version     string
	Source      struct{ Kind, Commit, Scope string }
	ModuleEdges []struct{ Importer, Target, Evidence string }
}
type lensFactsGeneration struct {
	Format   string
	Depth    int
	Parent   string
	Metadata lensFactsMetadata
	Upserts  map[string]string
	Deleted  []string
}
type lensFactsDB struct {
	metadata lensFactsMetadata
	files    map[string]lensFactsFile
}

// Cache reads are bounded and checksummed; paths are never taken from artifact IDs.
func lensFactsRead(path, id string, target any, remaining *int64) bool {
	if id != "" && (len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, *remaining+1))
	*remaining -= int64(len(b))
	if err != nil || *remaining < 0 {
		return false
	}
	if id != "" {
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != id {
			return false
		}
	}
	return json.Unmarshal(b, target) == nil
}
func lensFactsLoad(cache, id string, remaining *int64) (lensFactsDB, bool) {
	chain := []lensFactsGeneration{}
	for id != "" {
		var g lensFactsGeneration
		if len(chain) >= 32 || !lensFactsRead(filepath.Join(cache, "facts-data", "generations", id+".json"), id, &g, remaining) || g.Format != "facts-generation-v1" || g.Depth < 0 || g.Depth >= 32 || (g.Depth == 0) != (g.Parent == "") || g.Upserts == nil || g.Deleted == nil {
			return lensFactsDB{}, false
		}
		if len(chain) > 0 && chain[len(chain)-1].Depth != g.Depth+1 {
			return lensFactsDB{}, false
		}
		chain = append(chain, g)
		id = g.Parent
	}
	if len(chain) == 0 {
		return lensFactsDB{}, false
	}
	refs := map[string]string{}
	for i := len(chain) - 1; i >= 0; i-- {
		for _, p := range chain[i].Deleted {
			delete(refs, p)
		}
		for p, h := range chain[i].Upserts {
			refs[p] = h
		}
	}
	if len(refs) > 65536 {
		return lensFactsDB{}, false
	}
	db := lensFactsDB{metadata: chain[0].Metadata, files: map[string]lensFactsFile{}}
	paths := make([]string, 0, len(refs))
	for p := range refs {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		var f lensFactsFile
		h := refs[p]
		if !lensFactsRead(filepath.Join(cache, "facts-data", "objects", h+".json"), h, &f, remaining) || f.Path != p || f.Sha == "" || f.Symbols == nil {
			return lensFactsDB{}, false
		}
		db.files[p] = f
	}
	return db, true
}
func lensFactsTree(ctx context.Context, root string, m lensFactsMetadata) string {
	if m.Version != "1.3.0" || m.Source.Kind != "commit" || m.Source.Scope != "." || !validReviewGitTree(m.Source.Commit) {
		return ""
	}
	// Only the cache's full immutable commit ID is resolved: no HEAD, ref, or worktree read.
	cmd := exec.CommandContext(ctx, "git", "--no-replace-objects", "-C", root, "rev-parse", "--verify", m.Source.Commit+"^{tree}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
func lensFactsExports(f lensFactsFile) map[string]string {
	out := map[string]string{}
	for _, s := range f.Symbols {
		if s.IsExported {
			out[s.Name] = s.Kind + " " + s.Signature
		}
	}
	return out
}
func reviewLensFactsDigest(ctx context.Context, root string, frozen reviewtransaction.FrozenCandidateContext) string {
	if root == "" {
		return ""
	}
	cache := filepath.Join(root, ".pi", "facts-commit-cache")
	remaining := int64(32 << 20)
	var pointer struct{ Format, Generation string }
	if !lensFactsRead(filepath.Join(cache, "facts.json"), "", &pointer, &remaining) || pointer.Format != "facts-pointer-v1" {
		return ""
	}
	current, ok := lensFactsLoad(cache, pointer.Generation, &remaining)
	if !ok || lensFactsTree(ctx, root, current.metadata) != frozen.CandidateTree {
		return ""
	}
	// A baseline must itself be tree-pinned; an arbitrary previous generation is not evidence.
	var g lensFactsGeneration
	if !lensFactsRead(filepath.Join(cache, "facts-data", "generations", pointer.Generation+".json"), pointer.Generation, &g, &remaining) {
		return ""
	}
	var base lensFactsDB
	found := false
	for id := g.Parent; id != ""; {
		candidate, ok := lensFactsLoad(cache, id, &remaining)
		if !ok {
			return ""
		}
		if lensFactsTree(ctx, root, candidate.metadata) == frozen.BaseTree {
			base = candidate
			found = true
			break
		}
		if !lensFactsRead(filepath.Join(cache, "facts-data", "generations", id+".json"), id, &g, &remaining) {
			return ""
		}
		id = g.Parent
	}
	if !found {
		return ""
	}
	changed := map[string]bool{}
	paths := []string{}
	for index, p := range frozen.ChangedPathManifest {
		oldID, newID, ok := frozen.CandidatePathObjectIDs(index)
		if !ok {
			return ""
		}
		before, bok := base.files[p.Path]
		after, aok := current.files[p.Path]
		present := func(id string) bool { return strings.Trim(id, "0") != "" }
		if bok != present(oldID) || aok != present(newID) || (bok && before.Sha != oldID) || (aok && after.Sha != newID) {
			return ""
		}
		changed[p.Path] = true
		paths = append(paths, p.Path)
	}
	slices.Sort(paths)
	var out strings.Builder
	out.WriteString("Facts subject digest\n")
	for _, p := range paths {
		before, bok := base.files[p]
		after, aok := current.files[p]
		if !bok && !aok {
			return ""
		} // Unsupported/omitted sources cannot support a complete digest.
		status := "modified"
		if !bok {
			status = "added"
		} else if !aok {
			status = "deleted"
		}
		old, new := lensFactsExports(before), lensFactsExports(after)
		added, removed, modified := []string{}, []string{}, []string{}
		for name, sig := range new {
			if prev, ok := old[name]; !ok {
				added = append(added, name)
			} else if prev != sig {
				modified = append(modified, name)
			}
		}
		for name := range old {
			if _, ok := new[name]; !ok {
				removed = append(removed, name)
			}
		}
		slices.Sort(added)
		slices.Sort(removed)
		slices.Sort(modified)
		edges := 0
		dependents := map[string]bool{}
		for _, e := range current.metadata.ModuleEdges {
			if e.Target == "" || e.Evidence == "unresolved" {
				continue
			}
			if (e.Importer == p || e.Target == p) && changed[e.Importer] != changed[e.Target] {
				edges++
			}
			if e.Target == p && !changed[e.Importer] {
				dependents[e.Importer] = true
			}
		}
		a, _ := json.Marshal(added)
		r, _ := json.Marshal(removed)
		m, _ := json.Marshal(modified)
		fmt.Fprintf(&out, "%q %s exports +%s -%s changed=%s cross-boundary-edges=%d unchanged-dependents=%d\n", p, status, a, r, m, edges, len(dependents))
	}
	return out.String()
}
