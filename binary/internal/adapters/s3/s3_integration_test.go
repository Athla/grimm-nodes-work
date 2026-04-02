//go:build integration

package s3

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/testcontainers/testcontainers-go/modules/minio"

	"binary/internal/adapters"
	"binary/internal/adapters/adaptertest"
)

var (
	testAdapter adapters.Adapter
	testConfig  adapters.ConnectionConfig
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	container, err := minio.Run(ctx, "minio/minio:latest",
		minio.WithUsername("minioadmin"),
		minio.WithPassword("minioadmin"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start minio container: %v\n", err)
		os.Exit(1)
	}
	defer container.Terminate(ctx)

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get minio endpoint: %v\n", err)
		os.Exit(1)
	}

	// Seed: create buckets and objects with prefixes
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", ""),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load aws config: %v\n", err)
		os.Exit(1)
	}

	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.BaseEndpoint = aws.String("http://" + endpoint)
		o.UsePathStyle = true
	})

	buckets := []string{"test-bucket-1", "test-bucket-2"}
	for _, b := range buckets {
		if _, err := client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: aws.String(b)}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to create bucket %s: %v\n", b, err)
			os.Exit(1)
		}
	}

	// Add objects with prefixes to test-bucket-1
	prefixes := []string{"data/file1.txt", "logs/file2.txt"}
	for _, key := range prefixes {
		if _, err := client.PutObject(ctx, &awss3.PutObjectInput{
			Bucket: aws.String("test-bucket-1"),
			Key:    aws.String(key),
			Body:   strings.NewReader("content"),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "failed to put object %s: %v\n", key, err)
			os.Exit(1)
		}
	}

	// Connect the adapter under test
	testConfig = adapters.ConnectionConfig{
		"region":            "us-east-1",
		"access_key_id":     "minioadmin",
		"secret_access_key": "minioadmin",
		"endpoint":          "http://" + endpoint,
	}
	testAdapter = New()
	if err := testAdapter.Connect(testConfig); err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect adapter: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testAdapter.Close()
	os.Exit(code)
}

func TestContract(t *testing.T) {
	adaptertest.RunContractTests(t, testAdapter, func() adapters.Adapter { return New() }, testConfig, adaptertest.ContractOpts{
		MinNodes:           4, // 2 buckets + 2 prefixes
		MinEdges:           2, // 2 contains (prefixes in test-bucket-1)
		RootNodeType:       "bucket",
		ChildNodeTypes:     []string{"storage"},
		MultiRoot:          true,
		SkipConnectMissing: true,
		RequiredHealthKeys: []string{
			"status",
			"bucket_count",
		},
	})
}

func TestDiscover_BucketNodeIDs(t *testing.T) {
	allNodes, _, err := testAdapter.Discover()
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}

	for _, n := range allNodes {
		if !strings.HasPrefix(n.Id, "s3-") {
			t.Errorf("node ID %q does not start with \"s3-\"", n.Id)
		}
	}
}

func TestHealth_BucketCount(t *testing.T) {
	metrics, err := testAdapter.Health()
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}

	count, ok := metrics["bucket_count"].(int)
	if !ok {
		t.Fatal("bucket_count missing or not an int")
	}
	if count < 2 {
		t.Errorf("bucket_count = %d, want at least 2", count)
	}
}
