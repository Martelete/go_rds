package main

import (
	"strings"
	"testing"
)

func TestRenderReport(t *testing.T) {
	got := renderReport([]reportRow{{
		InstanceID:    "db-1",
		FreeStorage:   12.3456,
		EstMemUsed:    67.89,
		CPUAvg:        1.23,
		InstanceClass: "db.t3.micro",
	}})

	checks := []string{
		"RDS Monitoring Report\n",
		"Free storage is the lowest value in the last 24h. Estimated memory used is derived from FreeableMemory.",
		"Instance ID",
		"db-1",
		"12.35%",
		"67.89%",
		"1.23%",
		"db.t3.micro",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("report output missing %q\n--- got ---\n%q", check, got)
		}
	}
}
