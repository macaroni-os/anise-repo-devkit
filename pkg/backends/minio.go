/*
Copyright © 2020-2025 Macaroni OS Linux
See AUTHORS and LICENSE for the license details and contributors.
*/
package backends

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/macaroni-os/anise-repo-devkit/pkg/specs"

	artifact "github.com/geaaru/luet/pkg/v2/compiler/types/artifact"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type BackendMinio struct {
	Specs        *specs.AniseRDConfig
	ArtefactPath string

	MinioClient *minio.Client
	Bucket      string
}

func NewBackendMinio(specs *specs.AniseRDConfig, path string, opts map[string]string) (*BackendMinio, error) {

	if path != "" {
		_, err := os.Stat(path)
		if err != nil {
			return nil, errors.New(
				fmt.Sprintf(
					"Error on retrieve stat of the path %s: %s",
					path, err.Error(),
				))
		}

		if os.IsNotExist(err) {
			return nil, errors.New("The path doesn't exist!")
		}
	}

	if _, ok := opts["minio-bucket"]; !ok {
		return nil, errors.New("Minio bucket is mandatory")
	}

	if _, ok := opts["minio-endpoint"]; !ok {
		return nil, errors.New("Minio endpoint is mandatory")
	}

	if _, ok := opts["minio-keyid"]; !ok {
		return nil, errors.New("Minio key ID is mandatory")
	}

	if _, ok := opts["minio-secret"]; !ok {
		return nil, errors.New("Minio secret Access key is mandatory")
	}

	ans := &BackendMinio{
		Specs:        specs,
		ArtefactPath: path,
		Bucket:       opts["minio-bucket"],
	}

	minioRegion := ""
	minioSsl := true
	if _, ok := opts["minio-region"]; ok {
		minioRegion = opts["minio-region"]
	}

	if _, ok := opts["minio-ssl"]; ok {
		if opts["minio-ssl"] == "false" {
			minioSsl = false
		}
	}

	var mClient *minio.Client
	var err error

	mOpts := &minio.Options{
		Creds: credentials.NewStaticV4(
			opts["minio-keyid"],
			opts["minio-secret"],
			"",
		),
		Secure: minioSsl,
	}
	if minioRegion != "" {
		mOpts.Region = minioRegion
	}

	mClient, err = minio.New(
		opts["minio-endpoint"],
		mOpts,
	)
	if err != nil {
		return nil, errors.New("Error on create minio client: " + err.Error())
	}

	ans.MinioClient = mClient

	// Check if the bucket exists
	found, err := ans.MinioClient.BucketExists(context.Background(), ans.Bucket)
	if err != nil {
		return nil, errors.New(
			fmt.Sprintf("Error on check if the bucket %s: %s", ans.Bucket, err.Error()))
	}

	if !found {
		return nil, errors.New(fmt.Sprintf("Bucket %s not found", ans.Bucket))
	}

	return ans, nil
}

func (b *BackendMinio) GetFilesList() ([]string, error) {
	ans := []string{}
	opts := minio.ListObjectsOptions{
		Recursive: true,
		Prefix:    "",
	}

	// List all objects from a bucket-name with a matching prefix.
	for object := range b.MinioClient.ListObjects(context.Background(), b.Bucket, opts) {
		if object.Err != nil {
			return ans, errors.New("Error on retrieve list of objects: " + object.Err.Error())
		}

		ans = append(ans, object.Key)
	}

	return ans, nil
}

func (b *BackendMinio) GetMetadata(file string) (*artifact.PackageArtifact, error) {
	var outBuffer bytes.Buffer

	object, err := b.MinioClient.GetObject(
		context.Background(), b.Bucket, file, minio.GetObjectOptions{},
	)
	if err != nil {
		return nil, err
	}

	if _, err = io.Copy(&outBuffer, object); err != nil {
		return nil, err
	}

	fileContent := outBuffer.String()

	return artifact.NewPackageArtifactFromYaml([]byte(fileContent))
}

func (b *BackendMinio) CleanFile(file string) error {
	opts := minio.RemoveObjectOptions{
		GovernanceBypass: true,
	}
	return b.MinioClient.RemoveObject(context.Background(),
		b.Bucket, file, opts)
}
