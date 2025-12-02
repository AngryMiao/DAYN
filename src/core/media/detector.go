package media

import "strings"

// DetectFileSuffix 检测文件后缀
// 根据文件内容的魔数（magic number）检测文件类型
func DetectFileSuffix(data []byte, fileType string) string {
	if len(data) < 12 {
		return ""
	}

	fileType = strings.ToLower(fileType)

	switch fileType {
	case "image":
		return detectImageSuffix(data)
	case "video":
		return detectVideoSuffix(data)
	case "audio":
		return detectAudioSuffix(data)
	default:
		return ""
	}
}

// detectImageSuffix 检测图片文件后缀
func detectImageSuffix(data []byte) string {
	if len(data) < 12 {
		return ""
	}

	// JPEG: FF D8 FF
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpg"
	}

	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "png"
	}

	// GIF: 47 49 46 38 (GIF8)
	if len(data) >= 6 &&
		data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 &&
		(data[4] == 0x37 || data[4] == 0x39) && data[5] == 0x61 {
		return "gif"
	}

	// BMP: 42 4D
	if data[0] == 0x42 && data[1] == 0x4D {
		return "bmp"
	}

	// WebP: 52 49 46 46 ... 57 45 42 50
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "webp"
	}

	// TIFF: 49 49 2A 00 (little-endian) or 4D 4D 00 2A (big-endian)
	if len(data) >= 4 {
		if (data[0] == 0x49 && data[1] == 0x49 && data[2] == 0x2A && data[3] == 0x00) ||
			(data[0] == 0x4D && data[1] == 0x4D && data[2] == 0x00 && data[3] == 0x2A) {
			return "tiff"
		}
	}

	return ""
}

// detectVideoSuffix 检测视频文件后缀
func detectVideoSuffix(data []byte) string {
	if len(data) < 12 {
		return ""
	}

	// MP4: 00 00 00 [size] 66 74 79 70 (ftyp)
	if len(data) >= 8 {
		// 检查 ftyp box
		if data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 {
			// 检查具体的品牌
			if len(data) >= 12 {
				brand := string(data[8:12])
				// MP4 brands
				if strings.HasPrefix(brand, "mp4") || strings.HasPrefix(brand, "isom") ||
					strings.HasPrefix(brand, "M4V") || strings.HasPrefix(brand, "M4A") {
					return "mp4"
				}
				// MOV brands
				if strings.HasPrefix(brand, "qt") {
					return "mov"
				}
			}
			return "mp4" // 默认返回mp4
		}
	}

	// AVI: 52 49 46 46 ... 41 56 49 20
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x41 && data[9] == 0x56 && data[10] == 0x49 && data[11] == 0x20 {
		return "avi"
	}

	// FLV: 46 4C 56
	if data[0] == 0x46 && data[1] == 0x4C && data[2] == 0x56 {
		return "flv"
	}

	// MKV/WebM: 1A 45 DF A3
	if len(data) >= 4 &&
		data[0] == 0x1A && data[1] == 0x45 && data[2] == 0xDF && data[3] == 0xA3 {
		return "mkv"
	}

	// MPEG: 00 00 01 BA (MPEG-PS) or 00 00 01 B3 (MPEG-ES)
	if len(data) >= 4 &&
		data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 &&
		(data[3] == 0xBA || data[3] == 0xB3) {
		return "mpeg"
	}

	return ""
}

// detectAudioSuffix 检测音频文件后缀
func detectAudioSuffix(data []byte) string {
	if len(data) < 12 {
		return ""
	}

	// MP3: FF FB or FF F3 or FF F2 or ID3
	if (data[0] == 0xFF && (data[1] == 0xFB || data[1] == 0xF3 || data[1] == 0xF2)) ||
		(data[0] == 0x49 && data[1] == 0x44 && data[2] == 0x33) {
		return "mp3"
	}

	// WAV: 52 49 46 46 ... 57 41 56 45
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x41 && data[10] == 0x56 && data[11] == 0x45 {
		return "wav"
	}

	// FLAC: 66 4C 61 43
	if len(data) >= 4 &&
		data[0] == 0x66 && data[1] == 0x4C && data[2] == 0x61 && data[3] == 0x43 {
		return "flac"
	}

	// OGG: 4F 67 67 53
	if len(data) >= 4 &&
		data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53 {
		return "ogg"
	}

	// AAC: FF F1 or FF F9
	if data[0] == 0xFF && (data[1] == 0xF1 || data[1] == 0xF9) {
		return "aac"
	}

	// M4A (AAC in MP4 container): 检查 ftyp box
	if len(data) >= 12 &&
		data[4] == 0x66 && data[5] == 0x74 && data[6] == 0x79 && data[7] == 0x70 {
		brand := string(data[8:12])
		if strings.HasPrefix(brand, "M4A") {
			return "m4a"
		}
	}

	// AMR: 23 21 41 4D 52
	if len(data) >= 5 &&
		data[0] == 0x23 && data[1] == 0x21 && data[2] == 0x41 &&
		data[3] == 0x4D && data[4] == 0x52 {
		return "amr"
	}

	// Opus (in Ogg container)
	if len(data) >= 36 &&
		data[0] == 0x4F && data[1] == 0x67 && data[2] == 0x67 && data[3] == 0x53 &&
		data[28] == 0x4F && data[29] == 0x70 && data[30] == 0x75 && data[31] == 0x73 {
		return "opus"
	}

	return ""
}
