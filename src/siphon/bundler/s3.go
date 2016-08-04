package bundler

import (
	"log"
	"os"

	"gopkg.in/amz.v3/aws"
	_s3 "gopkg.in/amz.v3/s3"
)

const maxAttempts = 3

// Returns a bucket name according to whether we're in testing, staging or
// production
func getBucketName() string {
	env := os.Getenv("SIPHON_ENV")
	if env == "" || env == "testing" {
		return "siphon-files-testing"
	} else if env == "staging" {
		return "siphon-files-staging"
	} else {
		return "siphon-files"
	}
}

func makeS3() *_s3.S3 {
	auth := aws.Auth{AccessKey: os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretKey: os.Getenv("AWS_SECRET_ACCESS_KEY")}
	return _s3.New(auth, aws.USEast)
}

// CreateBuckets idempotently creates all of our required buckets
func CreateBuckets() error {
	name := getBucketName() // only one bucket for now
	s3 := makeS3()
	bucket, err := s3.Bucket(name)
	err = bucket.PutBucket(_s3.Private)
	if err != nil {
		return err
	}
	return nil
}

type S3Wrapper struct {
	s3     *_s3.S3
	bucket *_s3.Bucket
}

func NewS3Wrapper() *S3Wrapper {
	return &S3Wrapper{s3: makeS3()}
}

func (w *S3Wrapper) Open() error {
	bucket, err := w.s3.Bucket(getBucketName())
	if err != nil {
		return err
	}
	w.bucket = bucket
	return nil
}

func (w *S3Wrapper) WriteKey(key string, b []byte) error {
	log.Printf("[s3-write: %s]", key)
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = w.bucket.Put(key, b, "application/octet-stream", _s3.Private)
		if err == nil {
			return nil
		}
		log.Printf("[s3-write: %s -- err=%v, retrying]", key, err)
	}
	return err
}

func (w *S3Wrapper) DeleteKey(key string) error {
	log.Printf("[s3-delete: %s]", key)
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = w.bucket.Del(key)
		if err == nil {
			return nil
		}
		log.Printf("[s3-delete: %s -- err=%v, retrying]", key, err)
	}
	return err
}

func (w *S3Wrapper) GetKey(key string) (b []byte, err error) {
	log.Printf("[s3-get: %s]", key)
	for i := 0; i < maxAttempts; i++ {
		b, err = w.bucket.Get(key)
		if err == nil {
			return b, nil
		}
		log.Printf("[s3-get: %s -- err=%v, retrying]", key, err)
	}
	return nil, err
}
