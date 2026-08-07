package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateThenValidateRefusesIncompleteDraft(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "detections.ndjson")
	output := filepath.Join(directory, "drafts.json")
	detection := `{"schemaVersion":"1.0","startSecond":742,"endSecond":754,"score":0.83,"reasonTags":["objective-steal"],"signals":{"winProbabilitySwing":0.88,"eventDensity":0.81,"entityProximity":0.18,"resourceAsymmetry":0.72},"semanticEvidence":{"oneVersusManyUnitIds":[],"successfulEscapeUnitIds":[],"teamFightReversalSecond":null}}`
	if err := os.WriteFile(input, []byte(detection+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"create", "--input", input, "--output", output}); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"version": "2.1"`) || !strings.Contains(string(data), `"category": "objective-contest"`) {
		t.Fatalf("draft output did not preserve the expected contract: %s", data)
	}
	err = run([]string{"validate", "--draft", output})
	if err == nil || !strings.Contains(err.Error(), "analyst rationale is incomplete") {
		t.Fatalf("validate did not refuse the incomplete draft: %v", err)
	}
}
