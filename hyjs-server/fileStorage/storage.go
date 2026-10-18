package fileStorage

import (
	"context"
	"errors"
	"os"
	"time"
)

type Storage interface {
	GpUploadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error)
	GpDownloadUrl(ctx context.Context, bucketName, fileName string, expired time.Duration) (string, error)
	UploadFile(ctx context.Context, bucketName string, file *os.File) (string, error)
	DownloadFile(ctx context.Context, bucketName, fileName string) ([]byte, error)
	DownloadFileLocal(ctx context.Context, bucketName, fileName, savePath string) error
}

var fileStorage Storage

func GetStorage() Storage {
	return fileStorage
}

func Init(name string) error {
	if name == "cos" {
		fileStorage = initCosStorage()
		return nil
	}
	if name == "oss" {
		fileStorage = initOssStorage()
		return nil
	}
	return errors.New("not support storage")
}
