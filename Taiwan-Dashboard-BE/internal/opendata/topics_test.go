package opendata

import "testing"

func TestEveryDatasetHasATopic(t *testing.T) {
	known := map[string]bool{}
	for _, tp := range Topics {
		known[tp.ID] = true
	}
	used := map[string]bool{}
	for _, d := range Datasets {
		tp, ok := topicOf[d.ID]
		if !ok {
			t.Errorf("dataset %s has no topic", d.ID)
			continue
		}
		if !known[tp] {
			t.Errorf("dataset %s has unknown topic %q", d.ID, tp)
		}
		used[tp] = true
	}
	for id := range topicOf {
		if Find(id) == nil {
			t.Errorf("topicOf lists %s, which is not a dataset", id)
		}
	}
	for _, tp := range Topics {
		if !used[tp.ID] {
			t.Errorf("topic %s has no dataset", tp.ID)
		}
	}
}
