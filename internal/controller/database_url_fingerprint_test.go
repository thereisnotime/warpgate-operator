package controller

import (
	"crypto/sha256"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

func TestDatabaseURLFingerprint(t *testing.T) {
	inst := &warpgatev1alpha1.WarpgateInstance{ObjectMeta: metav1.ObjectMeta{UID: "uid-1"}}
	if got := databaseURLFingerprint(inst); got != nil {
		t.Fatalf("expected nil fingerprint without a database URL, got %x", got)
	}

	inst.Spec.DatabaseURL = "postgres://user:hunter2@db:5432/warpgate"
	a := databaseURLFingerprint(inst)
	if len(a) != 32 {
		t.Fatalf("expected 32-byte fingerprint, got %d", len(a))
	}
	raw := sha256.Sum256([]byte(inst.Spec.DatabaseURL))
	if string(a) == string(raw[:]) {
		t.Fatal("fingerprint must not be a plain SHA-256 of the URL")
	}
	if string(a) != string(databaseURLFingerprint(inst)) {
		t.Fatal("fingerprint must be deterministic")
	}

	before := configHash(inst)
	inst.Spec.DatabaseURL = "postgres://user:rotated@db:5432/warpgate"
	if databaseURLFingerprint(inst) == nil || string(databaseURLFingerprint(inst)) == string(a) {
		t.Fatal("fingerprint must change when the URL changes")
	}
	if after := configHash(inst); after == before {
		t.Fatal("configHash must change when the database URL changes")
	}
	if strings.Contains(configHash(inst), "rotated") {
		t.Fatal("configHash must not contain the URL")
	}
}
