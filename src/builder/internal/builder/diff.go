package builder

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Limits on what a diff prints. A dataset diff is read in a pull request
// comment, where a hundred records of detail is the same as none — the reader
// stops. The totals above stay exact; only the detail is elided, and the elision
// says how much it left out so a truncated diff cannot read as a complete one.
const (
	maxRecordsShown = 20
	maxFieldsShown  = 8
)

// Diff compares two built dataset trees and writes a summary of what changed.
//
// It exists because "data/ is unchanged" and "data/ changed in exactly this
// way" are the two claims a change to the builder has to support, and until now
// both were made by hand. A refactor that is supposed to produce an identical
// dataset, or a derivation that replaces an authored value with a computed one,
// is only reviewable if something recomputes both sides and says.
//
// It compares built output rather than committed files on purpose: a builder
// change alters what the overrides *would* produce, which a diff of what was
// committed cannot see — data/ may simply be stale.
//
// The comparison is over parsed YAML rather than the model, so a field added to
// a record is compared the day it is added rather than the day someone
// remembers to extend this.
func (a *App) Diff(baseDir string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	headDir := filepath.Join(a.Dir, cfg.Settings.DataDir)

	base, err := loadDataset(baseDir)
	if err != nil {
		return fmt.Errorf("read base dataset: %w", err)
	}
	head, err := loadDataset(headDir)
	if err != nil {
		return fmt.Errorf("read current dataset: %w", err)
	}

	d := compareDatasets(base, head)
	fmt.Fprint(a.Out, d.render())
	return nil
}

// record is one dataset file, parsed.
type record struct {
	path string
	tree any
}

// loadDataset reads every YAML record under dir, keyed by its path relative to
// dir so two trees in different directories compare.
//
// index.tsv is skipped: it is generated from the records by a separate command,
// so reporting it would restate every change already listed with none of the
// detail.
func loadDataset(dir string) (map[string]record, error) {
	out := map[string]record{}
	err := filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var tree any
		if err := yaml.Unmarshal(raw, &tree); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[rel] = record{path: rel, tree: tree}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// change is one field that differs, named by its path within the record.
type change struct {
	path   string
	before string
	after  string
}

// recordDiff is what changed within one record.
type recordDiff struct {
	path    string
	changes []change
}

// datasetDiff is the whole comparison.
type datasetDiff struct {
	compared int
	added    []string
	removed  []string
	changed  []recordDiff
}

// compareDatasets pairs the two trees by path and diffs each record.
func compareDatasets(base, head map[string]record) datasetDiff {
	d := datasetDiff{compared: len(head)}
	for path := range base {
		if _, ok := head[path]; !ok {
			d.removed = append(d.removed, path)
		}
	}
	for path, h := range head {
		b, ok := base[path]
		if !ok {
			d.added = append(d.added, path)
			continue
		}
		if changes := diffTrees("", b.tree, h.tree); len(changes) > 0 {
			d.changed = append(d.changed, recordDiff{path: path, changes: changes})
		}
	}
	sort.Strings(d.added)
	sort.Strings(d.removed)
	sort.Slice(d.changed, func(i, j int) bool { return d.changed[i].path < d.changed[j].path })
	return d
}

// diffTrees walks two parsed YAML values in step, returning the leaves that
// differ, each named by the path that reaches it.
//
// Lists are compared by position rather than matched up by identity. An
// inserted season shifts everything after it and reports as a long run of
// changes, which reads worse than "one season was inserted" but never claims a
// value changed when it did not — and getting identity right would mean knowing
// which key names an element, per list, which is the model knowledge this
// deliberately does without.
func diffTrees(path string, base, head any) []change {
	switch b := base.(type) {
	case map[string]any:
		h, ok := head.(map[string]any)
		if !ok {
			return []change{{path, format(base), format(head)}}
		}
		var out []change
		for _, k := range unionKeys(b, h) {
			bv, inBase := b[k]
			hv, inHead := h[k]
			switch {
			case !inBase:
				out = append(out, added(join(path, k), hv)...)
			case !inHead:
				out = append(out, removed(join(path, k), bv)...)
			default:
				out = append(out, diffTrees(join(path, k), bv, hv)...)
			}
		}
		return out
	case []any:
		h, ok := head.([]any)
		if !ok {
			return []change{{path, format(base), format(head)}}
		}
		var out []change
		for i := 0; i < max(len(b), len(h)); i++ {
			at := fmt.Sprintf("%s[%d]", path, i)
			switch {
			case i >= len(b):
				out = append(out, added(at, h[i])...)
			case i >= len(h):
				out = append(out, removed(at, b[i])...)
			default:
				out = append(out, diffTrees(at, b[i], h[i])...)
			}
		}
		return out
	default:
		if reflect.DeepEqual(base, head) {
			return nil
		}
		return []change{{path, format(base), format(head)}}
	}
}

// added and removed flatten a whole subtree that appeared or disappeared into
// its leaves, so a new block reads as the values it introduced rather than as
// "{1 fields}" — which names the shape and hides the fact.
func added(path string, v any) []change {
	out := make([]change, 0, 1)
	for _, l := range leaves(path, v) {
		out = append(out, change{l.path, "", l.after})
	}
	return out
}

func removed(path string, v any) []change {
	out := make([]change, 0, 1)
	for _, l := range leaves(path, v) {
		out = append(out, change{l.path, l.after, ""})
	}
	return out
}

// leaves flattens a value to its scalar leaves, each carried in the `after`
// field for the caller to place on whichever side it belongs.
func leaves(path string, v any) []change {
	switch t := v.(type) {
	case map[string]any:
		var out []change
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, leaves(join(path, k), t[k])...)
		}
		return out
	case []any:
		var out []change
		for i, e := range t {
			out = append(out, leaves(fmt.Sprintf("%s[%d]", path, i), e)...)
		}
		return out
	default:
		return []change{{path: path, after: format(v)}}
	}
}

// unionKeys returns every key of either map, in a fixed order.
func unionKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]any{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

// join appends a key to a field path.
func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// format renders a value for one line of a diff, collapsing a composite to a
// shape rather than dumping it: a whole season printed inline buries the field
// that actually changed.
func format(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case map[string]any:
		return fmt.Sprintf("{%d fields}", len(t))
	case []any:
		return fmt.Sprintf("[%d items]", len(t))
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

// render writes the summary a reviewer reads.
func (d datasetDiff) render() string {
	var b strings.Builder
	if len(d.added) == 0 && len(d.removed) == 0 && len(d.changed) == 0 {
		fmt.Fprintf(&b, "dataset identical: %d records rebuilt and compared, none changed\n", d.compared)
		return b.String()
	}

	fmt.Fprintf(&b, "dataset diff: %d records compared — %d changed, %d added, %d removed\n",
		d.compared, len(d.changed), len(d.added), len(d.removed))

	for _, p := range d.added {
		fmt.Fprintf(&b, "\n+ %s (new record)\n", p)
	}
	for _, p := range d.removed {
		fmt.Fprintf(&b, "\n- %s (record gone)\n", p)
	}

	shown := d.changed
	if len(shown) > maxRecordsShown {
		shown = shown[:maxRecordsShown]
	}
	for _, r := range shown {
		fmt.Fprintf(&b, "\n%s\n", r.path)
		fields := r.changes
		if len(fields) > maxFieldsShown {
			fields = fields[:maxFieldsShown]
		}
		for _, c := range fields {
			switch {
			case c.before == "":
				fmt.Fprintf(&b, "  + %s: %s\n", c.path, c.after)
			case c.after == "":
				fmt.Fprintf(&b, "  - %s: %s\n", c.path, c.before)
			default:
				fmt.Fprintf(&b, "  ~ %s: %s → %s\n", c.path, c.before, c.after)
			}
		}
		if rest := len(r.changes) - len(fields); rest > 0 {
			fmt.Fprintf(&b, "  … and %d more field(s)\n", rest)
		}
	}
	if rest := len(d.changed) - len(shown); rest > 0 {
		fmt.Fprintf(&b, "\n… and %d more changed record(s)\n", rest)
	}
	return b.String()
}
