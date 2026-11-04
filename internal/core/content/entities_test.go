package content

import (
	"encoding/json"
	"strings"
	"testing"
)

func entityGuide(entities string) []byte {
	return []byte(`{"schema_version":1,"id":"t","title":"T","sections":[{"id":"1","slug":"one","title":"One","steps":[
 {"id":"a","text":"Talk to Veos to start X Marks the Spot.","items":["Steel axe",{"name":"120 noted planks","qty":120}],
  "complete_when":[{"type":"quest_state","quest":"X Marks the Spot","state":"FINISHED"}],
  "links":[{"type":"wiki","url":"https://oldschool.runescape.wiki/w/X_Marks_the_Spot"}],
  "entities":` + entities + `},
 {"id":"b","text":"B"}
]}]}`)
}

func TestEntityMentionsValid(t *testing.T) {
	g, err := ParseGuide("t", entityGuide(`[
  {"ref":"quest:x_marks_the_spot","source_text":"X Marks the Spot","field":"/complete_when/0/quest","method":"rule","match":"name"},
  {"ref":"quest:x_marks_the_spot","source_text":"X Marks the Spot","field":"/text","span":[22,38],"method":"text","match":"name"},
  {"ref":"quest:x_marks_the_spot","source_text":"https://oldschool.runescape.wiki/w/X_Marks_the_Spot","field":"/links/0/url","method":"link","match":"wiki_key"},
  {"ref":"item:1353","source_text":"Steel axe","field":"/items/0","method":"item","match":"name"},
  {"ref":"item:961","source_text":"120 noted planks","field":"/items/1","method":"item","match":"plural"},
  {"ref":"step:b","source_text":"Talk","field":"/text","method":"text","match":"id"},
  {"ref":"boss:zulrah","source_text":"Talk","field":"/text","method":"text","match":"name"}
 ]`))
	if err != nil {
		t.Fatalf("ParseGuide: %v", err)
	}
	st, _ := g.Step("a")
	if len(st.Entities) != 7 || st.Entities[4].Ref.String() != "item:961" {
		t.Fatalf("entities = %+v", st.Entities)
	}
	out, err := json.Marshal(st.Entities[1])
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"ref":"quest:x_marks_the_spot","source_text":"X Marks the Spot","field":"/text","span":[22,38],"method":"text","match":"name"}`; string(out) != want {
		t.Errorf("mention round-trip = %s", out)
	}
}

func TestEntityMentionsRejected(t *testing.T) {
	cases := map[string]string{
		"schema: bad ref syntax":     `{"ref":"quest:X Marks","source_text":"X","field":"/text","method":"text","match":"name"}`,
		"schema: url as ref":         `{"ref":"https://oldschool.runescape.wiki/w/X","source_text":"X","field":"/text","method":"text","match":"name"}`,
		"schema: bad method":         `{"ref":"item:1353","source_text":"Steel axe","field":"/items/0","method":"guess","match":"name"}`,
		"unknown entity":             `{"ref":"quest:not_a_real_quest","source_text":"X","field":"/text","method":"text","match":"name"}`,
		"unknown entity item:999999": `{"ref":"item:999999","source_text":"Steel axe","field":"/items/0","method":"item","match":"name"}`,
		"unknown step":               `{"ref":"step:zzz","source_text":"Talk","field":"/text","method":"text","match":"id"}`,
		"does not exist":             `{"ref":"item:1353","source_text":"Steel axe","field":"/items/9","method":"item","match":"name"}`,
		"is not in":                  `{"ref":"item:1353","source_text":"Iron axe","field":"/items/0","method":"item","match":"name"}`,
		"does not select":            `{"ref":"quest:x_marks_the_spot","source_text":"X Marks the Spot","field":"/text","span":[0,16],"method":"text","match":"name"}`,
	}
	for want, mention := range cases {
		_, err := ParseGuide("t", entityGuide("["+mention+"]"))
		issues, ok := AsErrors(err)
		if !ok {
			t.Errorf("%s: want Errors, got %v", want, err)
			continue
		}
		var msgs []string
		for _, is := range issues {
			msgs = append(msgs, is.Pointer+": "+is.Message)
		}
		joined := strings.Join(msgs, "\n")
		if !strings.HasPrefix(want, "schema:") && !strings.Contains(joined, want) {
			t.Errorf("%s: issues do not mention it:\n%s", want, joined)
		}
		if !strings.Contains(joined, "/entities/0") {
			t.Errorf("%s: issue does not point at the mention:\n%s", want, joined)
		}
	}
}

func TestLoadoutItemIDValidated(t *testing.T) {
	guide := func(id int) []byte {
		return []byte(`{"schema_version":1,"id":"t","title":"T","default_variant":"main","variants":[{"id":"main","title":"Main","sections":[{"id":"1","slug":"one","title":"One","segments":[
 {"id":"s","kind":"sequence","loadout":{"status":"partial","inventory":[{"name":"Steel axe","item_id":` + itoa(id) + `}],
   "source":{"method":"explicit","confidence":"high","step_ids":["a"]}},
  "source":{"method":"derived","confidence":"high","step_ids":["a"]},"steps":[{"id":"a","text":"A"}]}
]}]}]}`)
	}
	if _, err := ParseGuide("t", guide(1353)); err != nil {
		t.Fatalf("known item id rejected: %v", err)
	}
	_, err := ParseGuide("t", guide(999999))
	if err == nil || !strings.Contains(err.Error(), "unknown item id 999999") {
		t.Fatalf("unknown loadout item id accepted: %v", err)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
