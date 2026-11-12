package message

import (
	"fmt"
	"sort"
	"strings"
)

// TagSighting is one version index across the facets of a field.
// Facets maps the caller-chosen facet name (the key suffix after field+".") to that version's row.
type TagSighting struct {
	Index  int
	Facets map[string]TagOutput
}

// TagTerm is one search term. Key is the full tag key; Value may contain '='.
type TagTerm struct {
	Key   string
	Value string
}

// tagBefore reports whether a is strictly earlier than b.
// Order is storage timestamp, then tag number. A missing tag number is 0.
func tagBefore(a, b TagOutput) bool {
	if a.Timestamp != b.Timestamp {
		return a.Timestamp < b.Timestamp
	}
	return a.TagNumber < b.TagNumber
}

// LatestByKey returns the latest row for each full tag key.
// The winner is the greatest storage timestamp, then the greatest tag number.
// When both are equal, the later input row wins.
func LatestByKey(tags []TagOutput) map[string]TagOutput {
	out := make(map[string]TagOutput, len(tags))
	for _, t := range tags {
		prev, ok := out[t.Key]
		if !ok || !tagBefore(t, prev) {
			out[t.Key] = t
		}
	}
	return out
}

// FieldHistory groups tags whose keys start with field+"." and zips them by version index.
// Each key is sorted by storage timestamp, then tag number. An empty field returns no sightings.
// A facet that has no row at index n is absent from that sighting.
func FieldHistory(tags []TagOutput, field string) []TagSighting {
	if field == "" {
		return nil
	}
	prefix := field + "."
	groups := make(map[string][]TagOutput)
	var order []string
	for _, t := range tags {
		if !strings.HasPrefix(t.Key, prefix) || len(t.Key) == len(prefix) {
			continue
		}
		facet := t.Key[len(prefix):]
		if _, ok := groups[facet]; !ok {
			order = append(order, facet)
		}
		groups[facet] = append(groups[facet], t)
	}
	if len(groups) == 0 {
		return nil
	}
	maxN := 0
	for _, facet := range order {
		rows := append([]TagOutput(nil), groups[facet]...)
		sort.SliceStable(rows, func(i, j int) bool {
			return tagBefore(rows[i], rows[j])
		})
		groups[facet] = rows
		if len(rows) > maxN {
			maxN = len(rows)
		}
	}
	// Right-align so a facet added in a later batch is absent from earlier sightings.
	sightings := make([]TagSighting, maxN)
	for i := range sightings {
		sightings[i] = TagSighting{Index: i, Facets: make(map[string]TagOutput)}
	}
	for _, facet := range order {
		rows := groups[facet]
		offset := maxN - len(rows)
		for i, row := range rows {
			sightings[offset+i].Facets[facet] = row
		}
	}
	return sightings
}

// SuggestDelta classifies a new value against the previous one.
// No previous row is "initial", the same text is "confirmed", and any other change is "modified".
// Retracted and contradicted are caller-supplied. AppendTags does not call this.
func SuggestDelta(hasPrevious bool, previous, next string) string {
	if !hasPrevious {
		return "initial"
	}
	if previous == next {
		return "confirmed"
	}
	return "modified"
}

// TagSearchClauses formats one search clause per term.
// The first clause is boolean:or. Every later clause is boolean:and. One low: term per line.
func TagSearchClauses(terms []TagTerm) string {
	var b strings.Builder
	for i, term := range terms {
		if i > 0 {
			b.WriteByte('\n')
		}
		op := "and"
		if i == 0 {
			op = "or"
		}
		fmt.Fprintf(&b, "clause_type:S\tboolean:%s\tlow:%s=%s", op, term.Key, term.Value)
	}
	return b.String()
}
