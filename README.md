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
TODO

## Notes
TODO
