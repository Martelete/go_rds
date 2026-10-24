package main

import "testing"

func TestRenderReport(t *testing.T) {
	got := renderReport([]reportRow{
		{
			InstanceID:    "db-1",
			FreeStorage:   12.3456,
			EstMemUsed:    67.89,
			CPUAvg:        1.23,
			InstanceClass: "db.t3.micro",
		},
		{
			InstanceID:    "orders-production-replica-01",
			FreeStorage:   0.5,
			EstMemUsed:    100,
			CPUAvg:        0,
			InstanceClass: "db.r6g.large",
		},
	})

	want := `RDS Monitoring Report
=======================================================================================================
Free storage is the lowest value in the last 24h. Estimated memory used is derived from FreeableMemory.
=======================================================================================================
Instance ID                              Free Storage       Est. Mem Used      CPU Avg 5m         Instance Class
db-1                                     12.35%             67.89%             1.23%              db.t3.micro
orders-production-replica-01             0.50%              100.00%            0.00%              db.r6g.large
`

	if got != want {
		t.Fatalf("report mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
