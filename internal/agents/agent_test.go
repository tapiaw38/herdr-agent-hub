package agents

import "testing"

func TestMapStatus(t *testing.T) {
	for in, want := range map[string]string{"working": "WORKING", "blocked": "WAITING", "done": "DONE", "idle": "IDLE", "unknown": "UNKNOWN", "lost": "OFFLINE"} {
		if got := MapStatus(in); got != want {
			t.Fatalf("MapStatus(%q)=%q want %q", in, got, want)
		}
	}
}
func TestMergeKeepsLocalNameAndLiveIdentity(t *testing.T) {
	live := []Agent{{ID: "p1", PaneID: "w1:p1", Name: "Claude", Status: "WORKING"}}
	got := Merge(live, []Agent{{ID: "p1", Name: "Writing", Favorite: true}})
	if len(got) != 1 || got[0].Name != "Writing" || got[0].PaneID != "w1:p1" || !got[0].Favorite {
		t.Fatalf("merge=%+v", got)
	}
}
