# Golang CPU and Memory visibility for AWS RDS instances

The goal is to provide faster and more accessible visibility into the CPU and memory usage of AWS RDS instances, eliminating the need to manually navigate CloudWatch metrics in the AWS console.

## Usage

```bash
go run main.go
```

- Notes:
Remember to update the AWS region 
```golang
func main() {
	region := "" // Choose any AWS region
	ctx := context.TODO()
```

## Examples
```bash
RDS Monitoring Report
==================================================================================================
Instance ID                           FreeStorage     MemoryUsed      CPU Used      Instance Class
db-demo-01                            96.14           % 22.86         % 2.14        % db.m5.large
db-demo-02                            96.45           % 82.85         % 4.35        % db.t4g.micro
db-demo-03                            97.55           % 86.19         % 5.49        % db.t4g.micro
db-demo-04                            93.07           % 51.35         % 2.39        % db.m5.large
db-demo-05                            92.57           % 45.88         % 2.54        % db.m5.large
db-demo-06                            91.88           % 83.11         % 2.91        % db.t4g.micro
db-demo-07                            17.43           % 29.88         % 2.65        % db.m5.large
db-demo-08                            77.17           % 26.20         % 6.26        % db.t4g.medium
db-demo-08                            13.64           % 27.11         % 6.38        % db.t4g.medium
db-demo-10                            82.34           % 76.83         % 3.86        % db.t3.medium
```

## Notes
TODO
