package fileStorage

import (
	"context"
	"fmt"
	"hyjs-server/config"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type CosStorage struct {
	region       string
	appId        string
	secretId     string
	secretKey    string
	bucketClient map[string]*cos.Client
}

func initCosStorage() *CosStorage {
	return &CosStorage{
		region:       config.AppConfigIns.StorageConfig.CosRegion,
		appId:        config.AppConfigIns.StorageConfig.CosAppId,
		secretId:     config.AppConfigIns.StorageConfig.CosSecretId,
		secretKey:    config.AppConfigIns.StorageConfig.CosSecretKey,
		bucketClient: make(map[string]*cos.Client),
	}
}

func (c *CosStorage) getClient(bucketName string) *cos.Client {
	client, ok := c.bucketClient[bucketName]
	if ok {
		return client
	}

	u, _ := url.Parse(fmt.Sprintf("https://%v-%v.cos.%v.myqcloud.com", bucketName, c.appId, c.region))
	b := &cos.BaseURL{BucketURL: u}
	client = cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  c.secretId,
			SecretKey: c.secretKey,
		},
	})
	c.bucketClient[bucketName] = client

	return client
}

func (c *CosStorage) GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	presignedURL, err := c.getClient(bucketName).Object.GetPresignedURL(ctx, http.MethodPut, fileName, c.secretId, c.secretKey, expired, nil)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (c *CosStorage) GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error) {
	presignedURL, err := c.getClient(bucketName).Object.GetPresignedURL(ctx, http.MethodGet, fileName, c.secretId, c.secretKey, expired, nil)
	if err != nil {
		return "", err
	}
	return presignedURL.String(), nil
}

func (c *CosStorage) UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error) {
	opt := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{},
		ACLHeaderOptions:       &cos.ACLHeaderOptions{},
	}
	_, err := c.getClient(bucketName).Object.Put(ctx, path.Base(file.Name()), file, opt)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%v-%v.cos.%v.myqcloud.com/%s",
		bucketName, c.appId, c.region, url.PathEscape(path.Base(file.Name()))), nil
}

func (c *CosStorage) DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error) {
	resp, err := c.getClient(bucketName).Object.Get(ctx, fileName, nil)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return b, err
}

func (c *CosStorage) DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error {
	_, err := c.getClient(bucketName).Object.GetToFile(ctx, fileName, savePath, nil)
	if err != nil {
		return err
	}
	return nil
}
