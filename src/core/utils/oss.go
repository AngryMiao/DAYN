package utils

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSConfig OSS配置
type OSSConfig struct {
	Region          string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
}

// OSSUploader OSS上传器
type OSSUploader struct {
	bucket *oss.Bucket
	config *OSSConfig
}

// NewOSSUploader 创建OSS上传器
func NewOSSUploader(config *OSSConfig) (*OSSUploader, error) {
	// 创建 OSS Client
	client, err := oss.New(config.Endpoint, config.AccessKeyID, config.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建OSS客户端失败: %v", err)
	}

	// 获取 Bucket
	bucket, err := client.Bucket(config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("获取Bucket失败: %v", err)
	}

	return &OSSUploader{
		bucket: bucket,
		config: config,
	}, nil
}

// UploadBase64 上传base64编码的文件
func (u *OSSUploader) UploadBase64(base64Data, fileType, deviceID string) (string, error) {
	// 解码base64数据
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return "", fmt.Errorf("base64解码失败: %v", err)
	}

	// 生成文件路径
	objectKey := u.generateObjectKey(fileType, deviceID)

	// 上传到OSS
	err = u.bucket.PutObject(objectKey, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("上传到OSS失败: %v", err)
	}

	// 生成文件访问URL
	fileURL := u.generateFileURL(objectKey)
	return fileURL, nil
}

// UploadFile 上传本地文件到OSS指定路径
func (u *OSSUploader) UploadFile(localPath, ossPath string) (string, error) {
	// 上传本地文件到OSS
	err := u.bucket.PutObjectFromFile(ossPath, localPath)
	if err != nil {
		return "", fmt.Errorf("上传到OSS失败: %v", err)
	}

	// 生成文件访问URL
	fileURL := u.generateFileURL(ossPath)
	return fileURL, nil
}

// generateFileURL 生成文件访问URL
func (u *OSSUploader) generateFileURL(ossPath string) string {
	// 清理 endpoint
	endpoint := strings.TrimPrefix(u.config.Endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	// 生成URL: https://{bucket}.{endpoint}/{path}
	return fmt.Sprintf("https://%s.%s/%s", u.config.Bucket, endpoint, ossPath)
}

// generateObjectKey 生成对象存储路径
// 格式: media/{type}/{device_id}/{date}/{timestamp}_{random}.{ext}
func (u *OSSUploader) generateObjectKey(fileType, deviceID string) string {
	now := time.Now()
	date := now.Format("2006-01-02")
	timestamp := now.Unix()

	// 根据文件类型确定目录和扩展名
	var dir, ext string
	switch strings.ToLower(fileType) {
	case "image", "img", "photo":
		dir = "images"
		ext = "jpg"
	case "video":
		dir = "videos"
		ext = "mp4"
	default:
		dir = "media"
		ext = "bin"
	}

	// 生成路径: media/{dir}/{device_id}/{date}/{timestamp}.{ext}
	objectKey := filepath.Join("media", dir, deviceID, date, fmt.Sprintf("%d.%s", timestamp, ext))

	// 将Windows路径分隔符转换为Unix风格
	objectKey = strings.ReplaceAll(objectKey, "\\", "/")

	return objectKey
}
