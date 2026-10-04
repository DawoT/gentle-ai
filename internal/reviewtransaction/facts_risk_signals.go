package reviewtransaction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type FactsRiskSignals struct {
	TestsOnly, CoverageComplete             bool
	UnchangedDependents, SymbolSurfaceDelta int
}
type factsRiskFile struct {
	Path, Sha string
	Symbols   []struct {
		Name, Kind, Signature string
		IsExported            bool
	}
}
type factsRiskGeneration struct {
	Format   string
	Depth    int
	Parent   string
	Metadata struct {
		Version     string
		Source      struct{ Kind, Commit, Scope string }
		ModuleEdges []struct{ Importer, Target, Evidence, Specifier, Reason string }
	}
	Upserts map[string]string
	Deleted []string
}
type factsRiskDB struct {
	generation factsRiskGeneration
	files      map[string]factsRiskFile
}

func factsRiskID(id string) bool {
	return (len(id) == 40 || len(id) == 64) && strings.Trim(id, "0123456789abcdef") == ""
}
func factsRiskRead(p, id string, target any, budget *int64) bool {
	if id != "" && (len(id) != 64 || !factsRiskID(id)) {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, *budget+1))
	*budget -= int64(len(b))
	if err != nil || *budget < 0 {
		return false
	}
	h := sha256.Sum256(b)
	return (id == "" || hex.EncodeToString(h[:]) == id) && json.Unmarshal(b, target) == nil
}
func factsRiskLoad(cache, id string, budget *int64) (factsRiskDB, bool) {
	chain := []factsRiskGeneration{}
	for id != "" {
		var g factsRiskGeneration
		if len(chain) >= 32 || !factsRiskRead(filepath.Join(cache, "facts-data", "generations", id+".json"), id, &g, budget) || g.Format != "facts-generation-v1" || g.Depth < 0 || g.Depth >= 32 || (g.Depth == 0) != (g.Parent == "") || g.Upserts == nil || g.Deleted == nil {
			return factsRiskDB{}, false
		}
		if len(chain) > 0 && chain[len(chain)-1].Depth != g.Depth+1 {
			return factsRiskDB{}, false
		}
		chain = append(chain, g)
		id = g.Parent
	}
	if len(chain) == 0 {
		return factsRiskDB{}, false
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
		return factsRiskDB{}, false
	}
	db := factsRiskDB{chain[0], map[string]factsRiskFile{}}
	for p, h := range refs {
		var f factsRiskFile
		normalized, err := normalizeLogicalPath(p)
		if err != nil || normalized != p || !factsRiskRead(filepath.Join(cache, "facts-data", "objects", h+".json"), h, &f, budget) || f.Path != p || !factsRiskID(f.Sha) || f.Symbols == nil {
			return factsRiskDB{}, false
		}
		db.files[p] = f
	}
	return db, true
}
func factsRiskTree(root string, db factsRiskDB) string {
	m := db.generation.Metadata
	if m.Version != "1.3.0" || m.Source.Kind != "commit" || m.Source.Scope != "." || !factsRiskID(m.Source.Commit) {
		return ""
	}
	b, err := runGit(context.Background(), root, nil, nil, "rev-parse", "--verify", m.Source.Commit+"^{tree}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func factsRiskExports(f factsRiskFile) map[string]string {
	out := map[string]string{}
	for _, s := range f.Symbols {
		if s.IsExported {
			out[s.Name] = s.Kind + " " + s.Signature
		}
	}
	return out
}

// Accept only an unambiguous subset of builtin/package syntax. In particular,
// aliases, relative/absolute paths, traversal, URLs and malformed names remain
// advisory misses: they cannot prove that a local dependency is absent.
var factsRiskPackageSpecifier = regexp.MustCompile(`^(@[a-z0-9_-]+/)?[a-z0-9_-]+(/[a-zA-Z0-9_-]+)*$`)
var factsRiskBuiltinSpecifier = regexp.MustCompile(`^node:[a-z][a-z0-9_]*(/[a-z][a-z0-9_]*)*$`)

func factsRiskNonlocalSpecifier(specifier string) bool {
	return factsRiskBuiltinSpecifier.MatchString(specifier) || factsRiskPackageSpecifier.MatchString(specifier)
}

// Nonindexed resources are vertices, not synthetic Facts sources. Only exact
// relative filesystem JSON resolutions can be verified against the frozen tree.
func factsRiskResources(root, tree string, db factsRiskDB, changed map[string]bool) (map[string]bool, bool) {
	resources := map[string]bool{}
	for _, e := range db.generation.Metadata.ModuleEdges {
		if _, indexed := db.files[e.Target]; indexed || e.Target == "" {
			continue
		}
		normalized, err := normalizeLogicalPath(e.Target)
		if err != nil || normalized != e.Target || path.Ext(e.Target) != ".json" || changed[e.Target] || e.Evidence != "filesystem" ||
			(!strings.HasPrefix(e.Specifier, "./") && !strings.HasPrefix(e.Specifier, "../")) || strings.ContainsAny(e.Specifier, "\\\x00:") ||
			path.Join(path.Dir(e.Importer), e.Specifier) != e.Target {
			return nil, false
		}
		if _, ok := db.files[e.Importer]; !ok {
			return nil, false
		}
		resources[e.Target] = false
		if len(resources) > 65536 {
			return nil, false
		}
	}
	paths := make([]string, 0, len(resources))
	for p := range resources {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return resources, true
	}
	oids := make([]string, 0, len(paths))
	fixed := []string{"ls-tree", "-r", "-z", tree, "--"}
	for _, batch := range batchLiteralPathspecs(paths, gitArgvPrefixLength(root, fixed...)) {
		entries, err := runGit(context.Background(), root, nil, nil, append(append([]string{}, fixed...), batch...)...)
		if err != nil {
			return nil, false
		}
		for _, entry := range bytes.Split(entries, []byte{0}) {
			if len(entry) == 0 {
				continue
			}
			tab := bytes.IndexByte(entry, '\t')
			if tab < 0 {
				return nil, false
			}
			fields := strings.Fields(string(entry[:tab]))
			p := string(entry[tab+1:])
			seen, requested := resources[p]
			if !requested || seen || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !factsRiskID(fields[2]) {
				return nil, false
			}
			resources[p] = true
			oids = append(oids, fields[2])
		}
	}
	for _, verified := range resources {
		if !verified {
			return nil, false
		}
	}
	// Read in one bounded batch as well: a tree entry alone does not prove
	// that its blob exists or can be decoded from the object database.
	if _, err := batchBlobContents(context.Background(), root, oids, 32<<20); err != nil {
		return nil, false
	}
	return resources, true
}

// ReadFactsRiskSignals reads frozen evidence only; invalid caches are optional misses.
func ReadFactsRiskSignals(repoRoot, baseTree, candidateTree string, changedPaths []string) (*FactsRiskSignals, error) {
	if repoRoot == "" || !factsRiskID(baseTree) || !factsRiskID(candidateTree) || len(changedPaths) == 0 {
		return nil, nil
	}
	cache := filepath.Join(repoRoot, ".pi", "facts-commit-cache")
	budget := int64(32 << 20)
	var pointer struct{ Format, Generation string }
	if !factsRiskRead(filepath.Join(cache, "facts.json"), "", &pointer, &budget) || pointer.Format != "facts-pointer-v1" {
		return nil, nil
	}
	current, ok := factsRiskLoad(cache, pointer.Generation, &budget)
	if !ok || factsRiskTree(repoRoot, current) != candidateTree {
		return nil, nil
	}
	var base factsRiskDB
	for id := current.generation.Parent; id != ""; {
		db, ok := factsRiskLoad(cache, id, &budget)
		if !ok {
			return nil, nil
		}
		if factsRiskTree(repoRoot, db) == baseTree {
			base = db
			break
		}
		id = db.generation.Parent
	}
	if base.files == nil {
		return nil, nil
	}
	changed := map[string]bool{}
	result := &FactsRiskSignals{TestsOnly: true, CoverageComplete: true}
	for _, p := range changedPaths {
		changed[p] = true
		result.TestsOnly = result.TestsOnly && isTestRiskPath(p)
	}
	// Verify cached blobs and changed-path presence against both frozen trees.
	for _, side := range []struct {
		tree string
		db   factsRiskDB
	}{{baseTree, base}, {candidateTree, current}} {
		paths := append([]string{}, changedPaths...)
		for p := range side.db.files {
			if !changed[p] {
				paths = append(paths, p)
			}
		}
		blobs, err := treeBlobSizes(context.Background(), repoRoot, side.tree, paths)
		if err != nil || len(blobs) != len(side.db.files) {
			return nil, nil
		}
		for _, b := range blobs {
			if side.db.files[b.path].Sha != b.oid {
				return nil, nil
			}
		}
	}
	for _, p := range changedPaths {
		before, bok := base.files[p]
		after, aok := current.files[p]
		if !bok && !aok {
			return nil, nil
		}
		if isTestRiskPath(p) {
			continue
		}
		old, new := factsRiskExports(before), factsRiskExports(after)
		for n, s := range new {
			if old[n] != s {
				result.SymbolSurfaceDelta++
			}
			delete(old, n)
		}
		result.SymbolSurfaceDelta += len(old)
	}
	dependents := map[string]bool{}
	for _, side := range []struct {
		tree string
		db   factsRiskDB
	}{{baseTree, base}, {candidateTree, current}} {
		db := side.db
		resources, ok := factsRiskResources(repoRoot, side.tree, db, changed)
		if !ok || db.generation.Metadata.ModuleEdges == nil {
			return nil, nil
		}
		for _, e := range db.generation.Metadata.ModuleEdges {
			_, importerOK := db.files[e.Importer]
			_, targetOK := db.files[e.Target]
			targetOK = targetOK || resources[e.Target]
			if !importerOK {
				return nil, nil
			}
			if e.Evidence == "unresolved" && e.Target == "" && e.Reason == "module-not-found" && factsRiskNonlocalSpecifier(e.Specifier) {
				continue
			}
			if !targetOK || (e.Evidence != "filesystem" && e.Evidence != "typescript") {
				return nil, nil
			}
			if (changed[e.Importer] || changed[e.Target]) && (!isTestRiskPath(e.Importer) || !isTestRiskPath(e.Target)) {
				result.TestsOnly = false
			}
			if changed[e.Target] && !isTestRiskPath(e.Target) && !changed[e.Importer] {
				dependents[e.Importer] = true
			}
		}
	}
	result.UnchangedDependents = len(dependents)
	return result, nil
}
