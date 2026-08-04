package engine

import "testing"

// --- extractDecisionJSON: the shared balanced-brace scan --------------------

func TestExtractDecisionJSONSingleLine(t *testing.T) {
	decision, ok := extractDecisionJSON(`{"action":"place","tower_type":"basic","position":[5,5]}`)
	if !ok {
		t.Fatalf("expected extraction to succeed")
	}
	if decision["action"] != "place" {
		t.Fatalf("expected action=place, got %v", decision["action"])
	}
}

func TestExtractDecisionJSONPrettyPrintedMultiLine(t *testing.T) {
	response := "{\n  \"action\": \"place\",\n  \"tower_type\": \"sniper\",\n  \"position\": [3, 4]\n}\n"
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected pretty-printed multi-line JSON to be extracted")
	}
	if decision["tower_type"] != "sniper" {
		t.Fatalf("expected tower_type=sniper, got %v", decision["tower_type"])
	}
}

func TestExtractDecisionJSONFenced(t *testing.T) {
	response := "```json\n{\"action\":\"spawn\",\"enemy_type\":\"tank\"}\n```"
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected fenced JSON to be extracted")
	}
	if decision["enemy_type"] != "tank" {
		t.Fatalf("expected enemy_type=tank, got %v", decision["enemy_type"])
	}
}

func TestExtractDecisionJSONFencedMultiLine(t *testing.T) {
	response := "Here is my move:\n```json\n{\n  \"action\": \"spawn\",\n  \"enemy_type\": \"healer\",\n  \"reason\": \"keep the tanks alive\"\n}\n```\nGood luck."
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected fenced multi-line JSON to be extracted")
	}
	if decision["enemy_type"] != "healer" {
		t.Fatalf("expected enemy_type=healer, got %v", decision["enemy_type"])
	}
}

func TestExtractDecisionJSONProseWrapped(t *testing.T) {
	response := `I will build a tower now. {"action":"place","tower_type":"buffer","position":[1,2]} That should help.`
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected prose-wrapped JSON to be extracted")
	}
	if decision["tower_type"] != "buffer" {
		t.Fatalf("expected tower_type=buffer, got %v", decision["tower_type"])
	}
}

// TestExtractDecisionJSONProseWithStrayBraces is the greedy-match trap: prose
// ahead of the real object contains its own, non-JSON brace pair (e.g. a
// clarifying aside like "the format is {like this}"). A naive `(?s)\{.*\}`
// greedy match would span from that stray '{' to the LAST '}' in the whole
// response, swallowing the real object into a blob that fails to parse. The
// balanced scan must instead reject the stray candidate and keep scanning
// for the real, valid object further down.
func TestExtractDecisionJSONProseWithStrayBraces(t *testing.T) {
	response := `Note: the schema looks like {this} but here is my actual move: {"action":"place","tower_type":"splash","position":[7,8]}`
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected extraction to find the real object past the stray braces")
	}
	if decision["action"] != "place" || decision["tower_type"] != "splash" {
		t.Fatalf("expected the real decision object, got %#v", decision)
	}
}

// TestExtractDecisionJSONTwoObjectsYieldsFirst guards the other half of the
// greedy-match trap: two complete JSON objects in one response must yield
// the first one, not a span across both (which is what `(?s)\{.*\}` would
// capture, and which would fail to parse as one object anyway).
func TestExtractDecisionJSONTwoObjectsYieldsFirst(t *testing.T) {
	response := `{"action":"place","tower_type":"basic","position":[1,1]} {"action":"place","tower_type":"sniper","position":[9,9]}`
	decision, ok := extractDecisionJSON(response)
	if !ok {
		t.Fatalf("expected extraction to succeed")
	}
	if decision["tower_type"] != "basic" {
		t.Fatalf("expected the FIRST object (tower_type=basic), got %#v", decision)
	}
}

func TestExtractDecisionJSONNoObjectFails(t *testing.T) {
	if _, ok := extractDecisionJSON("I think I should build something"); ok {
		t.Fatalf("expected extraction to fail when there is no JSON object at all")
	}
	if _, ok := extractDecisionJSON("```\nno json here\n```"); ok {
		t.Fatalf("expected extraction to fail on a fenced block with no JSON object")
	}
}

// --- parsers accept pretty-printed / multi-line JSON as model-authored -----

func TestParseTowerResponseAcceptsPrettyPrintedAsModelAuthored(t *testing.T) {
	response := "Sure thing, here's my move:\n```json\n{\n  \"action\": \"place\",\n  \"tower_type\": \"sniper\",\n  \"position\": [6, 6],\n  \"reason\": \"cover the choke point\"\n}\n```"
	got, err := (&OpenAIHandler{}).parseTowerResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["action"] != "place" || got["tower_type"] != "sniper" {
		t.Fatalf("expected the parsed decision, got %#v", got)
	}
	if src := takeDecisionSource(got); src != SourceModel {
		t.Fatalf("expected a pretty-printed decision to be untagged (SourceModel), got %q", src)
	}
}

func TestParseEnemyResponseAcceptsPrettyPrintedAsModelAuthored(t *testing.T) {
	response := "{\n  \"action\": \"spawn\",\n  \"enemy_type\": \"shielded\",\n  \"reason\": \"soak up sniper hits\"\n}\n"
	got, err := (&GeminiHandler{}).parseEnemyResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["action"] != "spawn" || got["enemy_type"] != "shielded" {
		t.Fatalf("expected the parsed decision, got %#v", got)
	}
	if src := takeDecisionSource(got); src != SourceModel {
		t.Fatalf("expected a pretty-printed decision to be untagged (SourceModel), got %q", src)
	}
}
