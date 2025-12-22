package models

import (
	"fmt"
	"strings"
)

// Tuple represents a relation tuple in the system
type Tuple struct {
	Namespace       string
	ObjectID        string
	Relation        string
	SubjectNamespace string
	SubjectID       string
	SubjectRelation string // Empty string "" for direct user subjects, set for usersets
}

// String returns a human-readable representation of the tuple
func (t *Tuple) String() string {
	if t.SubjectRelation != "" {
		return fmt.Sprintf("%s:%s#%s@%s:%s#%s",
			t.Namespace, t.ObjectID, t.Relation,
			t.SubjectNamespace, t.SubjectID, t.SubjectRelation)
	}
	return fmt.Sprintf("%s:%s#%s@%s:%s",
		t.Namespace, t.ObjectID, t.Relation,
		t.SubjectNamespace, t.SubjectID)
}

// Validate checks if the tuple is valid
func (t *Tuple) Validate() error {
	if strings.TrimSpace(t.Namespace) == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if strings.TrimSpace(t.ObjectID) == "" {
		return fmt.Errorf("object_id cannot be empty")
	}
	if strings.TrimSpace(t.Relation) == "" {
		return fmt.Errorf("relation cannot be empty")
	}
	if strings.TrimSpace(t.SubjectNamespace) == "" {
		return fmt.Errorf("subject_namespace cannot be empty")
	}
	if strings.TrimSpace(t.SubjectID) == "" {
		return fmt.Errorf("subject_id cannot be empty")
	}
	return nil
}

// IsUserset returns true if this tuple represents a userset (has subject_relation)
func (t *Tuple) IsUserset() bool {
	return t.SubjectRelation != ""
}

