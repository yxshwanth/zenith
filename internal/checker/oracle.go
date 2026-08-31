package checker

// VersionOp is one write in an independent version history (M1 oracle).
type VersionOp struct {
	Rev     uint64
	Key     string
	Present bool // false = tombstone
}

// VisibleAt reports whether key is present at revision r according to ops.
func VisibleAt(ops []VersionOp, key string, r uint64) bool {
	var latest *VersionOp
	for i := range ops {
		if ops[i].Key != key || ops[i].Rev > r {
			continue
		}
		latest = &ops[i]
	}
	if latest == nil {
		return false
	}
	return latest.Present
}

// GraphEdge is a tuple edge for the graph oracle (A1/A Phase-B).
type GraphEdge struct {
	ObjectNS, ObjectID, Relation string
	SubjectNS, SubjectID         string
	SubjectRel                   string // empty = direct
}

// RelRewrite describes Phase-B relation semantics for the oracle.
type RelRewrite struct {
	Kind     string   // "", "intersection", "exclusion", "ttu"
	Children []string // intersection/exclusion child relations
	Tupleset string   // ttu
	Computed string   // ttu
}

// Schema maps namespace → relation → rewrite.
type Schema map[string]map[string]RelRewrite

// GraphAllows is a brute-force union/direct reference evaluator — must not
// import internal/authz or internal/mvcc (docs/invariants.md).
func GraphAllows(edges []GraphEdge, ons, oid, rel, sns, sid string) bool {
	return GraphAllowsSchema(nil, edges, ons, oid, rel, sns, sid)
}

// GraphAllowsSchema evaluates with optional Phase-B rewrites.
func GraphAllowsSchema(schema Schema, edges []GraphEdge, ons, oid, rel, sns, sid string) bool {
	return graphWalk(schema, edges, ons, oid, rel, sns, sid, map[string]bool{}, 0)
}

func graphWalk(schema Schema, edges []GraphEdge, ons, oid, rel, sns, sid string, path map[string]bool, exclDepth int) bool {
	id := ons + "|" + oid + "|" + rel + "|" + sns + "|" + sid
	if path[id] {
		return false
	}
	path[id] = true
	defer delete(path, id)

	if rw, ok := schemaRel(schema, ons, rel); ok {
		switch rw.Kind {
		case "intersection":
			for _, child := range rw.Children {
				if !graphWalk(schema, edges, ons, oid, child, sns, sid, path, exclDepth) {
					return false
				}
			}
			return len(rw.Children) > 0
		case "exclusion":
			if exclDepth > 0 {
				return false // reject nested exclusion
			}
			if len(rw.Children) < 2 {
				return false
			}
			base := graphWalk(schema, edges, ons, oid, rw.Children[0], sns, sid, path, exclDepth+1)
			sub := graphWalk(schema, edges, ons, oid, rw.Children[1], sns, sid, path, exclDepth+1)
			return base && !sub
		case "ttu":
			for _, e := range edges {
				if e.ObjectNS != ons || e.ObjectID != oid || e.Relation != rw.Tupleset || e.SubjectRel != "" {
					continue
				}
				if graphWalk(schema, edges, e.SubjectNS, e.SubjectID, rw.Computed, sns, sid, path, exclDepth) {
					return true
				}
			}
			return false
		}
	}

	for _, e := range edges {
		if e.ObjectNS != ons || e.ObjectID != oid || e.Relation != rel {
			continue
		}
		if e.SubjectRel == "" {
			if e.SubjectNS == sns && e.SubjectID == sid {
				return true
			}
			continue
		}
		if graphWalk(schema, edges, e.SubjectNS, e.SubjectID, e.SubjectRel, sns, sid, path, exclDepth) {
			return true
		}
	}
	return false
}

func schemaRel(schema Schema, ns, rel string) (RelRewrite, bool) {
	if schema == nil {
		return RelRewrite{}, false
	}
	m, ok := schema[ns]
	if !ok {
		return RelRewrite{}, false
	}
	rw, ok := m[rel]
	return rw, ok
}
