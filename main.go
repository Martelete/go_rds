package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
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

func main() {
	region := "" // Choose any AWS region
	ctx := context.TODO()

	// Load AWS config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		log.Fatalf("Unable to load SDK config: %v", err)
	}

	// Create clients
	rdsClient := rds.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)

	// Get all DB instances
	rdsOutput, err := rdsClient.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})
	if err != nil {
		log.Fatalf("Failed to describe DB instances: %v", err)
	}

	if len(rdsOutput.DBInstances) == 0 {
		log.Println("No DB instances found in the region")
		return
	}

	// Time ranges
	now := time.Now().UTC()
	storageStart := now.Add(-24 * time.Hour)
	memoryStart := now.Add(-1 * time.Hour)
	cpuStart := now.Add(-1 * time.Hour)

	fmt.Println("RDS Monitoring Report")
	fmt.Println("==========================================================================")
	fmt.Printf("%-40s %-15s %-15s %-15s %-15s\n", "Instance ID", "Free Storage", "Memory Used", "CPU Used", "Instance Class")

	for _, db := range rdsOutput.DBInstances {
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
		storagePercent, err := getStoragePercent(cwClient, ctx, instanceID, storageStart, now, allocatedBytes)
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

		// Print results
		fmt.Printf("%-40s %-15.2f%% %-15.2f%% %-15.2f%% %-15s\n",
			instanceID, storagePercent, memPercent, cpuPercent, instanceClass)
	}
}

func getStoragePercent(cwClient *cloudwatch.Client, ctx context.Context, instanceID string, start, end time.Time, allocatedBytes int64) (float64, error) {
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
		Statistics: []types.Statistic{types.StatisticMaximum},
	})
	if err != nil {
		return 0, err
	}

	if len(output.Datapoints) == 0 {
		return 0, fmt.Errorf("no storage datapoints found")
	}

	maxFree := 0.0
	for _, dp := range output.Datapoints {
		if dp.Maximum != nil && *dp.Maximum > maxFree {
			maxFree = *dp.Maximum
		}
	}

	return (maxFree / float64(allocatedBytes)) * 100, nil
}

func getMemoryPercent(cwClient *cloudwatch.Client, ctx context.Context, instanceID string, start, end time.Time, totalMem uint64) (float64, error) {
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

	// Get latest datapoint
	latest := time.Time{}
	freeMem := 0.0
	for _, dp := range output.Datapoints {
		if dp.Timestamp.After(latest) && dp.Average != nil {
			latest = *dp.Timestamp
			freeMem = *dp.Average
		}
	}

	usedPercent := 100 - (freeMem/float64(totalMem))*100
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

	// Get latest datapoint
	latest := time.Time{}
	cpu := 0.0
	for _, dp := range output.Datapoints {
		if dp.Timestamp.After(latest) && dp.Average != nil {
			latest = *dp.Timestamp
			cpu = *dp.Average
		}
	}
	return cpu, nil
}
