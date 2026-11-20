package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func readS3Object(ctx context.Context, location string) ([]byte, error) {
	object, err := url.Parse(location)
	if err != nil {
		return nil, fmt.Errorf("parse S3 location: %w", err)
	}
	key := strings.TrimPrefix(object.Path, "/")
	if object.Hostname() == "" || object.Port() != "" || object.User != nil || key == "" ||
		object.RawQuery != "" || object.ForceQuery || object.Fragment != "" {
		return nil, fmt.Errorf("S3 location must be s3://bucket/key")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	response, err := s3.NewFromConfig(cfg).GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(object.Hostname()),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	return io.ReadAll(response.Body)
}
