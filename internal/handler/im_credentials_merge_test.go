package handler

import (
	"encoding/json"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestMergeIMCredentialsKeepsStoredSecrets(t *testing.T) {
	stored := types.JSON(`{"bot_token":"tok","ilink_bot_id":"bot","app_id":"old"}`)

	// The edit form cannot see stored secrets and submits blanks.
	got, err := mergeIMCredentials(stored, types.JSON(`{"bot_token":"","ilink_bot_id":null,"app_id":"new","extra":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"bot_token": "tok", "ilink_bot_id": "bot", "app_id": "new", "extra": "x"}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %v, want %v", k, m[k], v)
		}
	}

	// An empty object (what the form sends for an untouched WeChat channel) changes nothing.
	got, err = mergeIMCredentials(stored, types.JSON(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	m = nil
	_ = json.Unmarshal(got, &m)
	if m["bot_token"] != "tok" {
		t.Errorf("empty update wiped bot_token: %v", m)
	}

	if _, err := mergeIMCredentials(stored, types.JSON(`not json`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
