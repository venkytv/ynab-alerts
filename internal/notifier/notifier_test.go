package notifier

import "testing"

func TestBuildLogNotifier(t *testing.T) {
	n, err := Build(Options{Kind: "log"})
	if err != nil {
		t.Fatalf("expected log notifier, got error: %v", err)
	}
	if _, ok := n.(LogNotifier); !ok {
		t.Fatalf("expected LogNotifier, got %T", n)
	}
}

func TestBuildPushoverMissingCreds(t *testing.T) {
	_, err := Build(Options{Kind: "pushover"})
	if err == nil {
		t.Fatalf("expected error when pushover creds missing")
	}
}

func TestBuildUnknown(t *testing.T) {
	_, err := Build(Options{Kind: "bogus"})
	if err == nil {
		t.Fatalf("expected error on unknown notifier kind")
	}
}
