package s3

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/guilherme-grimm/graph-go/internal/adapters"
	"github.com/guilherme-grimm/graph-go/internal/graph/edges"
	"github.com/guilherme-grimm/graph-go/internal/graph/nodes"
)

var _ adapters.Adapter = (*adapter)(nil)

func init() {
	adapters.RegisterFactory("s3", func() adapters.Adapter { return New() })
}

type adapter struct {
	client *s3.Client
}

func New() *adapter {
	return &adapter{}
}

func (a *adapter) Connect(config adapters.ConnectionConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	region, _ := config["region"].(string)
	if region == "" {
		region = "us-east-1"
	}

	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(region))

	accessKey, _ := config["access_key_id"].(string)
	secretKey, _ := config["secret_access_key"].(string)
	if accessKey != "" && secretKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("s3: failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if endpoint, _ := config["endpoint"].(string); endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}

	a.client = s3.NewFromConfig(cfg, s3Opts...)

	// Verify connectivity
	_, err = a.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("s3: failed to list buckets (connectivity check): %w", err)
	}

	return nil
}

func (a *adapter) Discover() ([]nodes.Node, []edges.Edge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var allNodes []nodes.Node
	var allEdges []edges.Edge

	result, err := a.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, nil, fmt.Errorf("s3: failed to list buckets: %w", err)
	}

	for _, bucket := range result.Buckets {
		bucketName := aws.ToString(bucket.Name)
		bucketID := fmt.Sprintf("s3-%s", bucketName)

		allNodes = append(allNodes, nodes.Node{
			Id:   bucketID,
			Type: string(nodes.TypeBucket),
			Name: bucketName,
			Metadata: map[string]any{
				"adapter":    "s3",
				"created_at": bucket.CreationDate,
			},
			Health: "healthy",
		})

		// List top-level prefixes
		listResp, err := a.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:    bucket.Name,
			Delimiter: aws.String("/"),
		})
		if err != nil {
			continue // Skip buckets we can't read
		}

		for _, prefix := range listResp.CommonPrefixes {
			prefixName := aws.ToString(prefix.Prefix)
			prefixID := fmt.Sprintf("s3-%s-%s", bucketName, prefixName)

			allNodes = append(allNodes, nodes.Node{
				Id:       prefixID,
				Type:     string(nodes.TypeStorage),
				Name:     prefixName,
				Parent:   bucketID,
				Metadata: map[string]any{"adapter": "s3", "bucket": bucketName},
				Health:   "healthy",
			})

			allEdges = append(allEdges, edges.Edge{
				Id:     fmt.Sprintf("s3-contains-%s-%s", bucketName, prefixName),
				Source: bucketID,
				Target: prefixID,
				Type:   "contains",
				Label:  "contains",
			})
		}
	}

	return allNodes, allEdges, nil
}

func (a *adapter) Health() (adapters.HealthMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if a.client == nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": "not connected"}, nil
	}

	result, err := a.client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return adapters.HealthMetrics{"status": "unhealthy", "error": err.Error()}, nil
	}

	return adapters.HealthMetrics{
		"status":       "healthy",
		"bucket_count": len(result.Buckets),
	}, nil
}

func (a *adapter) Close() error {
	// S3 client has no persistent connection to close
	return nil
}
