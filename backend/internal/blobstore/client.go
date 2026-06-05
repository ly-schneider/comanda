package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// cacheControl marks uploaded assets as immutable; keys are content-addressed
// (random UUIDs) so the URL changes whenever the asset does.
const cacheControl = "public, max-age=31536000, immutable"

// Client wraps a Cloudflare R2 (S3-compatible) bucket. R2 requires path-style
// addressing and the "auto" region.
type Client struct {
	s3            *s3.Client
	bucket        string
	publicBaseURL string
}

// NewClient builds an R2-backed blob client. endpoint is the account endpoint
// (e.g. https://<accountid>.r2.cloudflarestorage.com); publicBaseURL is the
// public asset host the returned URLs are built from (custom domain or the
// r2.dev bucket URL).
func NewClient(endpoint, accessKeyID, secretAccessKey, bucket, publicBaseURL string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("blobstore: load config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	return &Client{
		s3:            client,
		bucket:        bucket,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
	}, nil
}

// Upload stores data under key and returns its public URL. key is the full
// object key (e.g. "products/<uuid>"); callers own the key namespace.
func (c *Client) Upload(ctx context.Context, key string, data io.ReadSeeker, contentType string) (string, error) {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(c.bucket),
		Key:          aws.String(key),
		Body:         data,
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(cacheControl),
	})
	if err != nil {
		return "", fmt.Errorf("blobstore: upload failed: %w", err)
	}
	return c.publicBaseURL + "/" + key, nil
}

// keyFromURL derives the object key from a public URL, or "" if the URL does
// not belong to this bucket's public base.
func (c *Client) keyFromURL(url string) string {
	prefix := c.publicBaseURL + "/"
	if !strings.HasPrefix(url, prefix) {
		return ""
	}
	return strings.TrimPrefix(url, prefix)
}

// Delete removes the object referenced by a public URL. URLs that don't belong
// to this bucket are ignored (no error), matching the previous behavior.
func (c *Client) Delete(ctx context.Context, url string) error {
	key := c.keyFromURL(url)
	if key == "" {
		return nil
	}
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("blobstore: delete failed: %w", err)
	}
	return nil
}

// EnsureBucket verifies the bucket is reachable. R2 buckets are provisioned out
// of band, so a missing bucket is logged by the caller rather than created here.
func (c *Client) EnsureBucket(ctx context.Context) error {
	_, err := c.s3.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			return fmt.Errorf("blobstore: bucket %q not reachable: %s", c.bucket, apiErr.ErrorCode())
		}
		return fmt.Errorf("blobstore: head bucket: %w", err)
	}
	return nil
}
