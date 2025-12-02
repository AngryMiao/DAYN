package media

import (
	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/utils"
	"fmt"
	"strings"
	"time"
)

// Uploader 媒体上传器
type Uploader struct {
	config *configs.Config
	logger *utils.Logger
}

// NewUploader 创建媒体上传器
func NewUploader(config *configs.Config, logger *utils.Logger) *Uploader {
	return &Uploader{
		config: config,
		logger: logger,
	}
}

// UploadRequest 上传请求
type UploadRequest struct {
	Base64Data string // base64编码的文件数据
	FileType   string // 文件类型：image/video/audio
	UserID     string // 用户ID
	DeviceID   string // 设备ID
}

// UploadResult 上传结果
type UploadResult struct {
	URL      string // 文件访问URL
	Path     string // OSS路径
	FileType string // 文件类型
	Suffix   string // 文件后缀
	Size     int64  // 文件大小
}

// Upload 上传媒体文件
func (u *Uploader) Upload(req *UploadRequest) (*UploadResult, error) {
	// 解码base64数据
	fileData, err := utils.DecodeBase64(req.Base64Data)
	if err != nil {
		return nil, fmt.Errorf("base64解码失败: %v", err)
	}

	// 检测文件类型和后缀
	fileSuffix := DetectFileSuffix(fileData, req.FileType)
	if fileSuffix == "" {
		return nil, fmt.Errorf("无法识别文件格式")
	}

	// 将 string 类型的 userID 转换为 uint
	userIDUint, err := utils.StringToUint(req.UserID)
	if err != nil {
		return nil, fmt.Errorf("用户ID转换失败: %v", err)
	}

	// 生成文件路径
	pathInfo := u.generateFilePath(userIDUint, req.FileType, fileSuffix)

	// 保存到本地
	localPath := fmt.Sprintf("uploads/%s", pathInfo.FullPath)
	if err := u.saveToLocal(localPath, fileData); err != nil {
		return nil, fmt.Errorf("保存本地文件失败: %v", err)
	}

	u.logger.Info("文件已保存到本地: %s", localPath)

	// 上传到OSS
	fileURL, err := u.uploadToOSS(localPath, pathInfo.FullPath)
	if err != nil {
		return nil, fmt.Errorf("上传到OSS失败: %v", err)
	}

	u.logger.Info("文件已上传到OSS: %s", fileURL)

	return &UploadResult{
		URL:      fileURL,
		Path:     pathInfo.FullPath,
		FileType: req.FileType,
		Suffix:   fileSuffix,
		Size:     int64(len(fileData)),
	}, nil
}

// PathInfo 路径信息
type PathInfo struct {
	EncryptedID string // 加密的用户ID
	Dir         string // 目录路径
	Filename    string // 文件名（不含后缀）
	FullPath    string // 完整路径
}

// generateFilePath 生成文件路径
func (u *Uploader) generateFilePath(userID uint, fileType, fileSuffix string) *PathInfo {
	// 生成加密的用户ID
	encryptedID := utils.HashUserIDWithSalt(userID, u.config.Server.Token, 12)

	// 生成随机文件名
	filename := utils.GenerateRandomKey(32)

	// 获取当前日期
	nowDate := time.Now().Format("20060102")

	// 生成目录路径
	dir := fmt.Sprintf("%s/%s/%s", encryptedID, fileType, nowDate)

	// 生成完整路径
	fullPath := fmt.Sprintf("%s/%s.%s", dir, filename, fileSuffix)

	return &PathInfo{
		EncryptedID: encryptedID,
		Dir:         dir,
		Filename:    filename,
		FullPath:    fullPath,
	}
}

// saveToLocal 保存文件到本地
func (u *Uploader) saveToLocal(localPath string, data []byte) error {
	// 提取目录路径
	lastSlash := strings.LastIndex(localPath, "/")
	if lastSlash > 0 {
		dir := localPath[:lastSlash]
		if err := utils.EnsureDir(dir); err != nil {
			return err
		}
	}

	return utils.WriteFile(localPath, data)
}

// uploadToOSS 上传文件到OSS
func (u *Uploader) uploadToOSS(localPath, ossPath string) (string, error) {
	ossConfig := u.config.OSS
	if ossConfig.AccessKeyID == "" || ossConfig.AccessKeySecret == "" {
		return "", fmt.Errorf("OSS配置不完整")
	}

	// 从endpoint提取region
	region := u.extractRegion(ossConfig.Endpoint)

	// 创建OSS上传器
	uploader, err := utils.NewOSSUploader(&utils.OSSConfig{
		Region:          region,
		Endpoint:        ossConfig.Endpoint,
		Bucket:          ossConfig.Bucket,
		AccessKeyID:     ossConfig.AccessKeyID,
		AccessKeySecret: ossConfig.AccessKeySecret,
	})
	if err != nil {
		return "", fmt.Errorf("创建OSS上传器失败: %v", err)
	}

	// 上传文件
	return uploader.UploadFile(localPath, ossPath)
}

// extractRegion 从endpoint提取region
func (u *Uploader) extractRegion(endpoint string) string {
	region := "cn-shenzhen" // 默认区域
	if strings.Contains(endpoint, "oss-") {
		parts := strings.Split(endpoint, "oss-")
		if len(parts) > 1 {
			regionPart := strings.Split(parts[1], ".")[0]
			region = regionPart
		}
	}
	return region
}
