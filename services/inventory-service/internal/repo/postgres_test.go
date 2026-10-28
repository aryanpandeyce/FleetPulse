package repo

import (
	"testing"
)

func TestPostgresRepoConstructor(t *testing.T) {
	r := NewPostgresRepo(nil)
	if r == nil {
		t.Fatalf("expected non-nil repo instance")
	}
}
