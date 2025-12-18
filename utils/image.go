package utils

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"kiro/types"
)

// SupportedImageFormats 支持的图片格式
var SupportedImageFormats = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

// MaxImageSize 最大图片大小 (20MB)
const MaxImageSize = 20 * 1024 * 1024

// DetectImageFormat 检测图片格式
func DetectImageFormat(data []byte) (string, error) {
	if len(data) < 12 {
		return "", fmt.Errorf("文件太小，无法检测格式")
	}

	// 检测 JPEG
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8 {
		return "image/jpeg", nil
	}

	// 检测 PNG
	if len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 &&
		data[4] == 0x0D && data[5] == 0x0A && data[6] == 0x1A && data[7] == 0x0A {
		return "image/png", nil
	}

	// 检测 GIF
	if len(data) >= 6 &&
		((data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x37 && data[5] == 0x61) ||
			(data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 && data[4] == 0x39 && data[5] == 0x61)) {
		return "image/gif", nil
	}

	// 检测 WebP
	if len(data) >= 12 &&
		data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46 &&
		data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
		return "image/webp", nil
	}

	// 检测 BMP
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return "image/bmp", nil
	}

	return "", fmt.Errorf("不支持的图片格式")
}

// ProcessImageData 处理图片数据，检测格式并编码为 base64
// IsSupportedImageFormat 检查是否为支持的图片格式
func IsSupportedImageFormat(mediaType string) bool {
	// 以 GetImageFormatFromMediaType 为单一事实来源，避免多处维护
	return GetImageFormatFromMediaType(mediaType) != ""
}

// GetImageFormatFromMediaType 从 media type 获取图片格式
func GetImageFormatFromMediaType(mediaType string) string {
	switch mediaType {
	case "image/jpeg":
		return "jpeg"
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/bmp":
		return "bmp"
	default:
		return ""
	}
}

// CreateCodeWhispererImage 创建 CodeWhisperer 格式的图片对象
func CreateCodeWhispererImage(imageSource *types.ImageSource) *types.CodeWhispererImage {
	if imageSource == nil {
		return nil
	}

	format := GetImageFormatFromMediaType(imageSource.MediaType)
	if format == "" {
		return nil
	}

	return &types.CodeWhispererImage{
		Format: format,
		Source: struct {
			Bytes string `json:"bytes"`
		}{
			Bytes: imageSource.Data,
		},
	}
}

// ParseImageFromContentBlock 从 ContentBlock 解析图片信息

// ValidateImageContent 验证图片内容的完整性
func ValidateImageContent(imageSource *types.ImageSource) error {
	if imageSource == nil {
		return fmt.Errorf("图片数据为空")
	}

	if imageSource.Type != "base64" {
		return fmt.Errorf("不支持的图片类型: %s", imageSource.Type)
	}

	if !IsSupportedImageFormat(imageSource.MediaType) {
		return fmt.Errorf("不支持的图片格式: %s", imageSource.MediaType)
	}

	if imageSource.Data == "" {
		return fmt.Errorf("图片数据为空")
	}

	// 验证 base64 编码并检查大小
	decodedData, err := base64.StdEncoding.DecodeString(imageSource.Data)
	if err != nil {
		return fmt.Errorf("无效的 base64 编码: %v", err)
	}

	if len(decodedData) > MaxImageSize {
		return fmt.Errorf("图片数据过大: %d 字节，最大支持 %d 字节", len(decodedData), MaxImageSize)
	}

	// 验证图片格式与数据是否匹配
	detectedType, err := DetectImageFormat(decodedData)
	if err == nil && detectedType != imageSource.MediaType {
		return fmt.Errorf("图片格式不匹配: 声明为 %s，实际为 %s", imageSource.MediaType, detectedType)
	}

	return nil
}

// ParseDataURL 解析data URL，提取媒体类型和base64数据
func ParseDataURL(dataURL string) (mediaType, base64Data string, err error) {
	// data URL格式：data:[<mediatype>][;base64],<data>
	dataURLPattern := regexp.MustCompile(`^data:([^;,]+)(;base64)?,(.+)$`)

	matches := dataURLPattern.FindStringSubmatch(dataURL)
	if len(matches) != 4 {
		return "", "", fmt.Errorf("无效的data URL格式")
	}

	mediaType = matches[1]
	isBase64 := matches[2] == ";base64"
	data := matches[3]

	if !isBase64 {
		return "", "", fmt.Errorf("仅支持base64编码的data URL")
	}

	// 验证是否为支持的图片格式
	if !IsSupportedImageFormat(mediaType) {
		return "", "", fmt.Errorf("不支持的图片格式: %s", mediaType)
	}

	// 验证base64编码
	decodedData, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", "", fmt.Errorf("无效的base64编码: %v", err)
	}

	// 检查文件大小
	if len(decodedData) > MaxImageSize {
		return "", "", fmt.Errorf("图片数据过大: %d 字节，最大支持 %d 字节", len(decodedData), MaxImageSize)
	}

	// 验证图片格式与声明是否匹配
	detectedType, err := DetectImageFormat(decodedData)
	if err == nil && detectedType != mediaType {
		return "", "", fmt.Errorf("图片格式不匹配: 声明为 %s，实际为 %s", mediaType, detectedType)
	}

	return mediaType, data, nil
}

// ConvertImageURLToImageSource 将 image_url 格式转换为 Anthropic 的 ImageSource 格式
func ConvertImageURLToImageSource(imageURL map[string]any) (*types.ImageSource, error) {
	// 获取URL字段
	urlValue, exists := imageURL["url"]
	if !exists {
		return nil, fmt.Errorf("image_url缺少url字段")
	}

	urlStr, ok := urlValue.(string)
	if !ok {
		return nil, fmt.Errorf("image_url的url字段必须是字符串")
	}

	// 检查是否是data URL
	if !strings.HasPrefix(urlStr, "data:") {
		return nil, fmt.Errorf("目前仅支持data URL格式的图片")
	}

	// 解析data URL
	mediaType, base64Data, err := ParseDataURL(urlStr)
	if err != nil {
		return nil, fmt.Errorf("解析data URL失败: %v", err)
	}

	return &types.ImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      base64Data,
	}, nil
}
