package telegram

import "testing"

func TestParseTextWithProfile(t *testing.T) {
	in := "p:alpha\nid:a1\nblob"
	got, ok := ParseText(in)
	if !ok || got.ProfileID != "alpha" || got.ID != "a1" || got.EncryptedData != "blob" {
		t.Fatalf("unexpected parse result: %+v ok=%v", got, ok)
	}
}

func TestParseTextNoProfile(t *testing.T) {
	in := "id:a1\nblob"
	got, ok := ParseText(in)
	if !ok || got.ID != "a1" || got.EncryptedData != "blob" {
		t.Fatalf("unexpected parse result: %+v ok=%v", got, ok)
	}
}
