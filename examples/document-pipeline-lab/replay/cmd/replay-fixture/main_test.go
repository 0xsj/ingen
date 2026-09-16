package main

import "testing"

func TestReadinessURLResolvesPathAgainstBaseURL(t *testing.T) {
	got, err := readinessURL("http://127.0.0.1:8080/api", "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:8080/api/healthz" {
		t.Fatalf("readiness URL = %q, want API path preserved", got)
	}
}

func TestReadinessURLRejectsRelativeInputs(t *testing.T) {
	if _, err := readinessURL("127.0.0.1:8080", "/healthz"); err == nil {
		t.Fatal("readinessURL accepted a relative base URL")
	}
	if _, err := readinessURL("http://127.0.0.1:8080", "healthz"); err == nil {
		t.Fatal("readinessURL accepted a relative readiness path")
	}
}
