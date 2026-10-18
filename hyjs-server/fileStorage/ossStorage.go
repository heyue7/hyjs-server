package fileStorage

import (
	"bytes"
	"context"
	"fmt"
	"hyjs-server/config"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type OssStorage struct {
	endPoint     string
	accessKey    string
	accessSecret string
	client       *oss.Client
}

func initOssStorage() *OssStorage {
	return &OssStorage{
		endPoint:     config.AppConfigIns.StorageConfig.OssEndPoint,
		accessKey:    config.AppConfigIns.StorageConfig.OssAccessKey,
		accessSecret: config.AppConfigIns.StorageConfig.OssAccessSecret,
	}
}

func (c *OssStorage) getClient() (*oss.Client, error) {
	if c.client != nil {
		return c.client, nil
	}
	client, err := oss.New(c.endPoint, c.accessKey, c.accessSecret)
	if err != nil {
		return nil, err
	}

	c.client = client
	return client, nil
}

func (c *OssStorage) GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	signURL, err := bucket.SignURL(fileName, oss.HTTPPut, int64(expired.Seconds()), oss.ContentType("text/plain;charset=utf8"))
	if err != nil {
		return "", err
	}
	return signURL, nil
}

func (c *OssStorage) GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	signURL, err := bucket.SignURL(fileName, oss.HTTPGet, int64(expired.Seconds()), oss.ContentType("text/plain;charset=utf8"))
	if err != nil {
		return "", err
	}
	return signURL, nil
}

func (c *OssStorage) UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error) {
	client, err := c.getClient()
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return "", err
	}

	objectKey := path.Base(file.Name())
	err = bucket.PutObject(objectKey, file)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimPrefix(strings.TrimPrefix(c.endPoint, "https://"), "http://")
	return fmt.Sprintf("https://%s.%s/%s", bucketName, endpoint, url.PathEscape(objectKey)), nil
}

func (c *OssStorage) DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error) {
	client, err := c.getClient()
	if err != nil {
		return nil, err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, err
	}

	body, err := bucket.GetObject(fileName)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, body)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *OssStorage) DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error {
	client, err := c.getClient()
	if err != nil {
		return err
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return err
	}

	err = bucket.GetObjectToFile(fileName, savePath)
	if err != nil {
		return err
	}

	return nil
}
