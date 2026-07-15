package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// Map of RDS instance class to total memory in bytes
var instanceClassToMemoryBytes = map[string]uint64{
	// T3 family
	"db.t3.micro":   1073741824,  // 1 GiB
	"db.t4g.micro":  1073741824,  // 1 GiB
	"db.t3.small":   2147483648,  // 2 GiB
	"db.t4g.small":  2147483648,  // 2 GiB
	"db.t3.medium":  4294967296,  // 4 GiB
	"db.t4g.medium": 4294967296,  // 4 GiB
	"db.t3.large":   8589934592,  // 8 GiB
	"db.t3.xlarge":  17179869184, // 16 GiB
	// M5 family
	"db.m5.large":   8589934592,  // 8 GiB
	"db.m5.xlarge":  17179869184, // 16 GiB
	"db.m5.2xlarge": 34359738368, // 32 GiB
	// R6g family
	"db.r6g.large":   17179869184, // 16 GiB
	"db.r6g.xlarge":  34359738368, // 32 GiB
	"db.r6g.2xlarge": 68719476736, // 64 GiB
	// Add more instance classes as needed
}

type reportRow struct {
	InstanceID    string
	FreeStorage   float64
	EstMemUsed    float64
	CPUAvg        float64
	InstanceClass string
}

func main() {
	var region string
	timeout := flag.Duration("timeout", 30*time.Second, "overall timeout for AWS API calls")
	flag.StringVar(&region, "region", "", "AWS region")
	flag.Parse()

	if region == "" {
		fmt.Print("Enter AWS region: ")
		_, err := fmt.Scanln(&region)
		if err != nil || region == "" {
			log.Fatal("valid region required")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Fatalf("Unable to load SDK config: %v", err)
	}

	// Create clients
	rdsClient := rds.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)

	// Get all DB instances across all pages.
	var dbInstances []rdstypes.DBInstance
	paginator := rds.NewDescribeDBInstancesPaginator(rdsClient, &rds.DescribeDBInstancesInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("Failed to describe DB instances: %v", err)
		}
		dbInstances = append(dbInstances, page.DBInstances...)
	}

	if len(dbInstances) == 0 {
		log.Println("No DB instances found in this region")
		return
	}

	// Time ranges
	now := time.Now().UTC()
	storageStart := now.Add(-24 * time.Hour)
	memoryStart := now.Add(-1 * time.Hour)
	cpuStart := now.Add(-1 * time.Hour)

	var rows []reportRow

	for _, db := range dbInstances {
		if db.DBInstanceIdentifier == nil || *db.DBInstanceIdentifier == "" {
			continue
		}
		instanceID := *db.DBInstanceIdentifier
		instanceClass := ""
		if db.DBInstanceClass != nil {
			instanceClass = *db.DBInstanceClass
		}

		// Skip if critical info is missing
		if db.AllocatedStorage == nil || instanceClass == "" {
			log.Printf("Skipping %s: missing allocated storage or instance class", instanceID)
			continue
		}

		// 1. Storage calculation
		allocatedBytes := int64(*db.AllocatedStorage) * 1024 * 1024 * 1024
		storagePercent, err := getMinFreeStoragePercent(cwClient, ctx, instanceID, storageStart, now, allocatedBytes)
		if err != nil {
			log.Printf("Storage error for %s: %v", instanceID, err)
			continue
		}

		// 2. Memory calculation
		memBytes, ok := instanceClassToMemoryBytes[instanceClass]
		if !ok {
			log.Printf("Skipping %s: unknown instance class %s", instanceID, instanceClass)
			continue
		}
		memPercent, err := getMemoryPercent(cwClient, ctx, instanceID, memoryStart, now, memBytes)
		if err != nil {
			log.Printf("Memory error for %s: %v", instanceID, err)
			continue
		}

		// 3. CPU calculation
		cpuPercent, err := getCPUPercent(cwClient, ctx, instanceID, cpuStart, now)
		if err != nil {
			log.Printf("CPU error for %s: %v", instanceID, err)
			continue
		}

		rows = append(rows, reportRow{
			InstanceID:    instanceID,
			FreeStorage:   storagePercent,
			EstMemUsed:    memPercent,
			CPUAvg:        cpuPercent,
			InstanceClass: instanceClass,
		})
	}

	fmt.Print(renderReport(rows))
}

func renderReport(rows []reportRow) string {
	var builder strings.Builder

	builder.WriteString("RDS Monitoring Report\n")
	builder.WriteString("=======================================================================================================\n")
	builder.WriteString("Free storage is the lowest value in the last 24h. Estimated memory used is derived from FreeableMemory.\n")
	builder.WriteString("=======================================================================================================\n")
	builder.WriteString(fmt.Sprintf("%-40s %-16s %-18s %-18s %-18s\n", "Instance ID", "Free Storage", "Est. Mem Used", "CPU Avg 5m", "Instance Class"))

	for _, row := range rows {
		builder.WriteString(fmt.Sprintf("%-40s %-18s %-18s %-15s %-15s\n",
			row.InstanceID,
			formatPercent(row.FreeStorage),
			formatPercent(row.EstMemUsed),
			formatPercent(row.CPUAvg),
			row.InstanceClass))
	}

	return builder.String()
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value)
}

func getMinFreeStoragePercent(cwClient *cloudwatch.Client, ctx context.Context, instanceID string, start, end time.Time, allocatedBytes int64) (float64, error) {
	if allocatedBytes <= 0 {
		return 0, fmt.Errorf("allocated storage must be greater than zero")
	}

	output, err := cwClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("FreeStorageSpace"),
		Dimensions: []types.Dimension{{
			Name:  aws.String("DBInstanceIdentifier"),
			Value: aws.String(instanceID),
		}},
		StartTime:  &start,
		EndTime:    &end,
		Period:     aws.Int32(3600),
		Statistics: []types.Statistic{types.StatisticMinimum},
	})
	if err != nil {
		return 0, err
	}

	if len(output.Datapoints) == 0 {
		return 0, fmt.Errorf("no storage datapoints found")
	}

	minFree := math.MaxFloat64
	for _, dp := range output.Datapoints {
		if dp.Minimum != nil && *dp.Minimum < minFree {
			minFree = *dp.Minimum
		}
	}

	if minFree == math.MaxFloat64 {
		return 0, fmt.Errorf("no valid minimum free storage data")
	}

	return (minFree / float64(allocatedBytes)) * 100, nil
}

func getMemoryPercent(cwClient *cloudwatch.Client, ctx context.Context, instanceID string, start, end time.Time, totalMem uint64) (float64, error) {
	if totalMem == 0 {
		return 0, fmt.Errorf("total memory must be greater than zero")
	}

	output, err := cwClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("FreeableMemory"),
		Dimensions: []types.Dimension{{
			Name:  aws.String("DBInstanceIdentifier"),
			Value: aws.String(instanceID),
		}},
		StartTime:  &start,
		EndTime:    &end,
		Period:     aws.Int32(300),
		Statistics: []types.Statistic{types.StatisticAverage},
	})
	if err != nil {
		return 0, err
	}

	if len(output.Datapoints) == 0 {
		return 0, fmt.Errorf("no memory datapoints found")
	}

	freeMem, err := latestAverage(output.Datapoints)
	if err != nil {
		return 0, err
	}

	usedPercent := 100 - (freeMem/float64(totalMem))*100
	if usedPercent < 0 {
		usedPercent = 0
	}
	if usedPercent > 100 {
		usedPercent = 100
	}
	return usedPercent, nil
}

func getCPUPercent(cwClient *cloudwatch.Client, ctx context.Context, instanceID string, start, end time.Time) (float64, error) {
	output, err := cwClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []types.Dimension{{
			Name:  aws.String("DBInstanceIdentifier"),
			Value: aws.String(instanceID),
		}},
		StartTime:  &start,
		EndTime:    &end,
		Period:     aws.Int32(300),
		Statistics: []types.Statistic{types.StatisticAverage},
	})
	if err != nil {
		return 0, err
	}

	if len(output.Datapoints) == 0 {
		return 0, fmt.Errorf("no CPU datapoints found")
	}

	cpu, err := latestAverage(output.Datapoints)
	if err != nil {
		return 0, err
	}

	return cpu, nil
}

func latestAverage(datapoints []types.Datapoint) (float64, error) {
	latest := time.Time{}
	value := 0.0
	found := false

	for _, dp := range datapoints {
		if dp.Timestamp == nil || dp.Average == nil {
			continue
		}
		if !found || dp.Timestamp.After(latest) {
			latest = *dp.Timestamp
			value = *dp.Average
			found = true
		}
	}

	if !found {
		return 0, fmt.Errorf("no valid datapoints found")
	}

	return value, nil
}
