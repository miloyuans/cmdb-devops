# AWS provider adapter

Implement this directory when replacing mock provider with live AWS SDK for Go v2.

Recommended packages:

- github.com/aws/aws-sdk-go-v2/config
- github.com/aws/aws-sdk-go-v2/credentials
- github.com/aws/aws-sdk-go-v2/service/ec2
- github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2
- github.com/aws/aws-sdk-go-v2/service/iam
- github.com/aws/aws-sdk-go-v2/service/cloudtrail

Expose a type that satisfies `cloud.Provider`, then register it in `internal/app/app.go`.
