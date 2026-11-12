package message

import (
	"strings"
	"testing"
)

func tagOut(key, value, ts string, n int) TagOutput {
	return TagOutput{Key: key, Value: value, Timestamp: ts, TagNumber: n, Frequency: 1}
}

func TestLatestByKey_TimestampThenTagNumber(t *testing.T) {
	tags := []TagOutput{
		tagOut("hue.alpha", "old", "0000000001.000000", 9),
		tagOut("hue.alpha", "mid", "0000000002.000000", 1),
		tagOut("hue.alpha", "tie-low", "0000000002.000000", 4),
		tagOut("hue.beta", "only", "0000000001.000000", 1),
	}
	// Equal timestamp: greater tag number wins, and a later equal row would win.
	// Insert the greater tag number after the lower one at the same timestamp.
	tags = append(tags, tagOut("hue.alpha", "tie-high", "0000000002.000000", 4))
	got := LatestByKey(tags)
	if got["hue.alpha"].Value != "tie-high" {
		t.Fatalf("hue.alpha = %q, want tie-high (same timestamp, later equal tag number)", got["hue.alpha"].Value)
	}
	if got["hue.beta"].Value != "only" {
		t.Fatalf("hue.beta = %q, want only", got["hue.beta"].Value)
	}

	later := LatestByKey([]TagOutput{
		tagOut("k", "low-number", "0000000005.000000", 1),
		tagOut("k", "high-number", "0000000005.000000", 8),
	})
	if later["k"].Value != "high-number" {
		t.Fatalf("latest = %q, want high-number", later["k"].Value)
	}
	earlierTs := LatestByKey([]TagOutput{
		tagOut("k", "new-ts", "0000000009.000000", 1),
		tagOut("k", "old-ts", "0000000001.000000", 99),
	})
	if earlierTs["k"].Value != "new-ts" {
		t.Fatalf("latest = %q, want new-ts", earlierTs["k"].Value)
	}
}

func TestFieldHistory_ZipAndEdges(t *testing.T) {
	tags := []TagOutput{
		tagOut("paint.beta", "b0", "0000000002.000000", 1),
		tagOut("paint.alpha", "a-low", "0000000002.000000", 2),
		tagOut("paint.alpha", "a-tie", "0000000002.000000", 9),
		tagOut("other.alpha", "nope", "0000000009.000000", 1),
		tagOut("paint", "bare", "0000000009.000000", 1),
	}
	got := FieldHistory(tags, "paint")
	if len(got) != 2 {
		t.Fatalf("sightings = %d, want 2", len(got))
	}
	if got[0].Index != 0 || got[0].Facets["alpha"].Value != "a-low" {
		t.Fatalf("index 0 alpha = %+v", got[0].Facets["alpha"])
	}
	if _, ok := got[0].Facets["beta"]; ok {
		t.Fatal("beta was absent from the first version and must be omitted at index 0")
	}
	if got[1].Facets["alpha"].Value != "a-tie" {
		t.Fatalf("index 1 alpha = %q, want a-tie (greater tag number)", got[1].Facets["alpha"].Value)
	}
	if got[1].Facets["beta"].Value != "b0" {
		t.Fatalf("index 1 beta = %q, want b0", got[1].Facets["beta"].Value)
	}
	if FieldHistory(tags, "") != nil {
		t.Fatal("empty field must return no sightings")
	}
	if FieldHistory(tags, "pai") != nil {
		t.Fatal("prefix without the separating dot must not match")
	}

	dotted := FieldHistory([]TagOutput{
		tagOut("a.b.c", "yes", "0000000001.000000", 1),
		tagOut("a.bc", "no", "0000000001.000000", 1),
		tagOut("a.b", "bare", "0000000001.000000", 1),
	}, "a.b")
	if len(dotted) != 1 || dotted[0].Facets["c"].Value != "yes" || len(dotted[0].Facets) != 1 {
		t.Fatalf("dotted field history = %+v", dotted)
	}
}

func TestFieldHistory_ConventionalFacetNamesAreOrdinaryKeys(t *testing.T) {
	tags := []TagOutput{
		tagOut("stage.value", "seed", "0000000001.000000", 1),
		tagOut("stage.event_timestamp", "2020", "0000000001.000001", 2),
		tagOut("stage.delta_type", "initial", "0000000001.000002", 3),
		tagOut("stage.confidence", "0.5", "0000000001.000003", 4),
		tagOut("stage.evidence", "", "0000000001.000004", 5),
		tagOut("stage.source", "doc-1", "0000000001.000005", 6),
		tagOut("stage.value", "series-a", "0000000002.000000", 7),
		tagOut("stage.event_timestamp", "2021", "0000000002.000001", 8),
		tagOut("stage.delta_type", "modified", "0000000002.000002", 9),
		tagOut("stage.confidence", "0.9", "0000000002.000003", 10),
		tagOut("stage.evidence", "span", "0000000002.000004", 11),
		tagOut("stage.source", "doc-2", "0000000002.000005", 12),
	}
	got := FieldHistory(tags, "stage")
	if len(got) != 2 {
		t.Fatalf("sightings = %d, want 2", len(got))
	}
	if got[0].Facets["value"].Value != "seed" || got[1].Facets["source"].Value != "doc-2" {
		t.Fatalf("conventional names = %+v / %+v", got[0].Facets, got[1].Facets)
	}
}

func TestSuggestDelta(t *testing.T) {
	if SuggestDelta(false, "", "seed") != "initial" {
		t.Fatal("missing previous")
	}
	if SuggestDelta(true, "seed", "seed") != "confirmed" {
		t.Fatal("same text")
	}
	if SuggestDelta(true, "seed", "series-a") != "modified" {
		t.Fatal("different text")
	}
}

func TestTagSearchClauses(t *testing.T) {
	got := TagSearchClauses([]TagTerm{
		{Key: "hue.alpha", Value: "red=blue"},
		{Key: "hue.beta", Value: "1"},
	})
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	if lines[0] != "clause_type:S\tboolean:or\tlow:hue.alpha=red=blue" {
		t.Fatalf("first = %q", lines[0])
	}
	if lines[1] != "clause_type:S\tboolean:and\tlow:hue.beta=1" {
		t.Fatalf("second = %q", lines[1])
	}
}

func TestDecodeGetEvent_LinkAndTargetTagHistory(t *testing.T) {
	header := "_type=get\t_status=OK\t_event_id=evt_a\t_unique_id=eo_seed"
	payload := strings.Join([]string{
		"_link=link1\tunique_id=lk_1\ttarget_event=evt_n\ttarget_unique_id=eo_nbr\tstrength=0.5\tcategory=rel",
		"_linktag\tevent_id=link1\tunique=lk_1\tfreq=1\ttimestamp=0000000002.000000\tvalue=paint.alpha=later",
		"_linktag\tevent_id=link1\tunique=lk_1\tfreq=1\ttimestamp=0000000001.000000\tvalue=paint.alpha=earlier",
		"_target_event_tag\tevent_id=evt_n\tfreq=1\ttimestamp=0000000002.000000\tvalue=paint.alpha=n-later",
		"_target_event_tag\tevent_id=evt_n\tfreq=1\ttimestamp=0000000001.000000\tvalue=paint.alpha=n-earlier",
	}, "\n")
	raw := buildMinimalMessage("mem@gateway.example.com", "client@gateway.example.com", header, 1001, 0, payload)
	decoded, err := DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if decoded.Event == nil || len(decoded.Event.Links) != 1 {
		t.Fatalf("links = %+v", decoded.Event)
	}
	link := decoded.Event.Links[0]
	if len(link.Tags) != 2 || link.Tags[0].Timestamp == "" || link.Tags[1].Timestamp == "" {
		t.Fatalf("link tags = %+v", link.Tags)
	}
	if len(link.TargetTags) != 2 || link.TargetTags[0].Timestamp == "" || link.TargetTags[1].Timestamp == "" {
		t.Fatalf("target tags = %+v", link.TargetTags)
	}
	linkHist := FieldHistory(link.Tags, "paint")
	if len(linkHist) != 2 || linkHist[0].Facets["alpha"].Value != "earlier" || linkHist[1].Facets["alpha"].Value != "later" {
		t.Fatalf("link history = %+v", linkHist)
	}
	targetHist := FieldHistory(link.TargetTags, "paint")
	if len(targetHist) != 2 || targetHist[0].Facets["alpha"].Value != "n-earlier" || targetHist[1].Facets["alpha"].Value != "n-later" {
		t.Fatalf("target history = %+v", targetHist)
	}
}
