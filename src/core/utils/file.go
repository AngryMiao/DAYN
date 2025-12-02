package utils

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
)

// DecodeBase64 解码base64字符串
func DecodeBase64(data string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("base64解码失败: %v", err)
	}
	return decoded, nil
}

// EnsureDir 确保目录存在，如果不存在则创建
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("创建目录失败: %v", err)
	}
	return nil
}

// WriteFile 写入文件
func WriteFile(path string, data []byte) error {
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入文件失败: %v", err)
	}
	return nil
}

// StringToUint 将字符串转换为uint
func StringToUint(s string) (uint, error) {
	val, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("字符串转换为uint失败: %v", err)
	}
	return uint(val), nil
}

// GetMimeType 根据文件类型和后缀获取 MIME 类型
func GetMimeType(fileType, suffix string) string {
	// 图片类型
	if fileType == "image" {
		switch suffix {
		case "jpg", "jpeg":
			return "image/jpeg"
		case "png":
			return "image/png"
		case "gif":
			return "image/gif"
		case "bmp":
			return "image/bmp"
		case "webp":
			return "image/webp"
		case "tiff":
			return "image/tiff"
		default:
			return "image/" + suffix
		}
	}

	// 视频类型
	if fileType == "video" {
		switch suffix {
		case "mp4":
			return "video/mp4"
		case "mov":
			return "video/quicktime"
		case "avi":
			return "video/x-msvideo"
		case "flv":
			return "video/x-flv"
		case "mkv":
			return "video/x-matroska"
		case "mpeg":
			return "video/mpeg"
		default:
			return "video/" + suffix
		}
	}

	// 音频类型
	if fileType == "audio" {
		switch suffix {
		case "mp3":
			return "audio/mpeg"
		case "wav":
			return "audio/wav"
		case "flac":
			return "audio/flac"
		case "ogg":
			return "audio/ogg"
		case "aac":
			return "audio/aac"
		case "m4a":
			return "audio/mp4"
		case "amr":
			return "audio/amr"
		case "opus":
			return "audio/opus"
		default:
			return "audio/" + suffix
		}
	}

	return "application/octet-stream"
}

// MediaMetadata 媒体文件元数据
type MediaMetadata struct {
	Width           *int     // 图片宽度
	Height          *int     // 图片高度
	DurationSeconds *float64 // 音频/视频时长（秒）
}

// GetImageDimensions 获取图片尺寸
func GetImageDimensions(data []byte) (width, height int, err error) {
	if len(data) < 24 {
		return 0, 0, fmt.Errorf("数据太短，无法解析图片尺寸")
	}

	// PNG: 89 50 4E 47
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		if len(data) < 24 {
			return 0, 0, fmt.Errorf("PNG数据不完整")
		}
		// PNG IHDR chunk 在偏移16处，宽高各4字节（大端序）
		width = int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
		height = int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
		return width, height, nil
	}

	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return parseJPEGDimensions(data)
	}

	// GIF: 47 49 46 38
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		if len(data) < 10 {
			return 0, 0, fmt.Errorf("GIF数据不完整")
		}
		// GIF 宽高在偏移6处，各2字节（小端序）
		width = int(data[6]) | int(data[7])<<8
		height = int(data[8]) | int(data[9])<<8
		return width, height, nil
	}

	// BMP: 42 4D
	if data[0] == 0x42 && data[1] == 0x4D {
		if len(data) < 26 {
			return 0, 0, fmt.Errorf("BMP数据不完整")
		}
		// BMP 宽高在偏移18处，各4字节（小端序）
		width = int(data[18]) | int(data[19])<<8 | int(data[20])<<16 | int(data[21])<<24
		height = int(data[22]) | int(data[23])<<8 | int(data[24])<<16 | int(data[25])<<24
		return width, height, nil
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(data) >= 30 && data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		// WebP VP8 格式
		if data[12] == 0x56 && data[13] == 0x50 && data[14] == 0x38 {
			if data[15] == 0x20 && len(data) >= 30 { // VP8
				width = int(data[26]) | int(data[27])<<8
				height = int(data[28]) | int(data[29])<<8
				return width & 0x3FFF, height & 0x3FFF, nil
			} else if data[15] == 0x4C && len(data) >= 25 { // VP8L
				bits := uint32(data[21]) | uint32(data[22])<<8 | uint32(data[23])<<16 | uint32(data[24])<<24
				width = int((bits & 0x3FFF) + 1)
				height = int(((bits >> 14) & 0x3FFF) + 1)
				return width, height, nil
			}
		}
	}

	return 0, 0, fmt.Errorf("不支持的图片格式")
}

// parseJPEGDimensions 解析JPEG图片尺寸
func parseJPEGDimensions(data []byte) (width, height int, err error) {
	offset := 2 // 跳过 FF D8
	for offset < len(data)-9 {
		// 查找标记
		if data[offset] != 0xFF {
			offset++
			continue
		}

		marker := data[offset+1]
		offset += 2

		// SOF0-SOF15 标记（除了SOF4, SOF8, SOF12）
		if (marker >= 0xC0 && marker <= 0xCF) && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if offset+5 >= len(data) {
				return 0, 0, fmt.Errorf("JPEG SOF数据不完整")
			}
			// 跳过长度(2字节)和精度(1字节)
			height = int(data[offset+3])<<8 | int(data[offset+4])
			width = int(data[offset+5])<<8 | int(data[offset+6])
			return width, height, nil
		}

		// 跳过当前段
		if offset+2 > len(data) {
			break
		}
		segmentLength := int(data[offset])<<8 | int(data[offset+1])
		offset += segmentLength
	}

	return 0, 0, fmt.Errorf("未找到JPEG尺寸信息")
}

// GetAudioDuration 获取音频时长（秒）
func GetAudioDuration(data []byte, suffix string) (float64, error) {
	switch suffix {
	case "wav":
		return parseWAVDuration(data)
	case "mp3":
		return parseMP3Duration(data)
	case "flac":
		return parseFLACDuration(data)
	case "m4a", "aac":
		return parseAACAudioDuration(data)
	default:
		return 0, fmt.Errorf("不支持的音频格式: %s", suffix)
	}
}

// parseWAVDuration 解析WAV音频时长
func parseWAVDuration(data []byte) (float64, error) {
	if len(data) < 44 {
		return 0, fmt.Errorf("WAV数据不完整")
	}

	// 检查RIFF头
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, fmt.Errorf("不是有效的WAV文件")
	}

	// 查找fmt chunk
	offset := 12
	for offset < len(data)-8 {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(data[offset+4]) | int(data[offset+5])<<8 | int(data[offset+6])<<16 | int(data[offset+7])<<24

		if chunkID == "fmt " && offset+8+chunkSize <= len(data) {
			// 读取采样率（偏移24-27）
			sampleRate := int(data[offset+12]) | int(data[offset+13])<<8 | int(data[offset+14])<<16 | int(data[offset+15])<<24
			// 读取字节率（偏移28-31）
			byteRate := int(data[offset+16]) | int(data[offset+17])<<8 | int(data[offset+18])<<16 | int(data[offset+19])<<24

			// 查找data chunk
			dataOffset := offset + 8 + chunkSize
			for dataOffset < len(data)-8 {
				dataChunkID := string(data[dataOffset : dataOffset+4])
				dataChunkSize := int(data[dataOffset+4]) | int(data[dataOffset+5])<<8 | int(data[dataOffset+6])<<16 | int(data[dataOffset+7])<<24

				if dataChunkID == "data" {
					if byteRate > 0 {
						duration := float64(dataChunkSize) / float64(byteRate)
						return duration, nil
					}
					if sampleRate > 0 {
						// 备用计算方法
						bitsPerSample := int(data[offset+22]) | int(data[offset+23])<<8
						channels := int(data[offset+10]) | int(data[offset+11])<<8
						duration := float64(dataChunkSize) / float64(sampleRate*channels*bitsPerSample/8)
						return duration, nil
					}
					return 0, fmt.Errorf("无法计算WAV时长")
				}
				dataOffset += 8 + dataChunkSize
			}
		}
		offset += 8 + chunkSize
	}

	return 0, fmt.Errorf("未找到WAV格式信息")
}

// parseMP3Duration 解析MP3音频时长（简化版本，基于文件大小和比特率估算）
func parseMP3Duration(data []byte) (float64, error) {
	if len(data) < 4 {
		return 0, fmt.Errorf("MP3数据不完整")
	}

	// 查找第一个MP3帧
	for i := 0; i < len(data)-4; i++ {
		if data[i] == 0xFF && (data[i+1]&0xE0) == 0xE0 {
			// 解析帧头
			version := (data[i+1] >> 3) & 0x03
			layer := (data[i+1] >> 1) & 0x03
			bitrateIndex := (data[i+2] >> 4) & 0x0F
			samplingRateIndex := (data[i+2] >> 2) & 0x03

			if version == 3 && layer == 1 && bitrateIndex > 0 && bitrateIndex < 15 {
				// MPEG1 Layer3 (MP3)
				bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
				sampleRates := []int{44100, 48000, 32000}

				if int(samplingRateIndex) < len(sampleRates) {
					bitrate := bitrates[bitrateIndex] * 1000 // 转换为bps
					duration := float64(len(data)*8) / float64(bitrate)
					return duration, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("未找到有效的MP3帧")
}

// parseFLACDuration 解析FLAC音频时长
func parseFLACDuration(data []byte) (float64, error) {
	if len(data) < 42 {
		return 0, fmt.Errorf("FLAC数据不完整")
	}

	// 检查FLAC标识
	if string(data[0:4]) != "fLaC" {
		return 0, fmt.Errorf("不是有效的FLAC文件")
	}

	// 读取STREAMINFO块（第一个元数据块）
	if (data[4] & 0x7F) == 0 { // STREAMINFO类型
		// 采样率在字节18-20（20位）
		sampleRate := (int(data[18]) << 12) | (int(data[19]) << 4) | (int(data[20]) >> 4)
		// 总样本数在字节21-25（36位）
		totalSamples := (int64(data[21]&0x0F) << 32) | (int64(data[22]) << 24) | (int64(data[23]) << 16) | (int64(data[24]) << 8) | int64(data[25])

		if sampleRate > 0 {
			duration := float64(totalSamples) / float64(sampleRate)
			return duration, nil
		}
	}

	return 0, fmt.Errorf("未找到FLAC STREAMINFO")
}

// parseAACAudioDuration 解析AAC/M4A音频时长（简化版本）
func parseAACAudioDuration(data []byte) (float64, error) {
	// AAC/M4A通常在MP4容器中，需要解析MP4结构
	// 这里提供简化版本，基于文件大小估算
	if len(data) < 8 {
		return 0, fmt.Errorf("AAC数据不完整")
	}

	// 检查是否是MP4容器
	if len(data) >= 12 && data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 {
		// 简化估算：假设128kbps比特率
		duration := float64(len(data)*8) / float64(128000)
		return duration, nil
	}

	return 0, fmt.Errorf("无法解析AAC时长")
}

// GetVideoDuration 获取视频时长（秒）- 简化版本
func GetVideoDuration(data []byte, suffix string) (float64, error) {
	// 视频时长解析比较复杂，这里提供基于文件大小的粗略估算
	// 实际生产环境建议使用ffmpeg等专业工具
	switch suffix {
	case "mp4", "mov":
		return parseMP4Duration(data)
	default:
		// 其他格式使用粗略估算（假设2Mbps比特率）
		duration := float64(len(data)*8) / float64(2000000)
		return duration, nil
	}
}

// parseMP4Duration 解析MP4视频时长（简化版本）
func parseMP4Duration(data []byte) (float64, error) {
	if len(data) < 8 {
		return 0, fmt.Errorf("MP4数据不完整")
	}

	// 查找mvhd box（Movie Header Box）
	offset := 0
	for offset < len(data)-8 {
		if offset+8 > len(data) {
			break
		}

		boxSize := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		boxType := string(data[offset+4 : offset+8])

		if boxSize < 8 || offset+boxSize > len(data) {
			break
		}

		if boxType == "mvhd" && offset+32 <= len(data) {
			// mvhd version 0
			if data[offset+8] == 0 {
				timescale := int(data[offset+20])<<24 | int(data[offset+21])<<16 | int(data[offset+22])<<8 | int(data[offset+23])
				duration := int(data[offset+24])<<24 | int(data[offset+25])<<16 | int(data[offset+26])<<8 | int(data[offset+27])
				if timescale > 0 {
					return float64(duration) / float64(timescale), nil
				}
			}
		}

		offset += boxSize
	}

	// 如果找不到mvhd，使用粗略估算
	duration := float64(len(data)*8) / float64(2000000) // 假设2Mbps
	return duration, nil
}
