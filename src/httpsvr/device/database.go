package device

import (
	"angrymiao-ai-server/src/configs/database"
	"angrymiao-ai-server/src/models"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// DeviceDB 设备绑定数据库操作结构体
type DeviceDB struct {
	db *gorm.DB
}

// NewDeviceDB 创建设备绑定数据库操作实例
func NewDeviceDB() *DeviceDB {
	return &DeviceDB{
		db: database.GetDB(),
	}
}

// SaveDevice 保存设备绑定信息
func (d *DeviceDB) SaveDevice(deviceID string, userID uint, bindKey string) error {
	// 检查设备是否已经绑定
	var existingBind models.Device
	err := d.db.Where("device_id = ? ", deviceID).First(&existingBind).Error
	if err == nil {
		// 设备已绑定，更新绑定信息
		existingBind.BindKey = bindKey
		existingBind.IsActive = true
		existingBind.UpdateAt = time.Now()
		return d.db.Save(&existingBind).Error
	} else if err != gorm.ErrRecordNotFound {
		return err
	}

	// 创建新的绑定记录
	// 注意：MacAddress 和 ClientID 有唯一约束，暂时使用 deviceID 作为占位符
	// 设备首次连接时会通过 UpdateDeviceStatus 更新真实的 MAC 地址和 ClientID
	newBind := models.Device{
		DeviceID:   deviceID,
		UserID:     userID,
		BindKey:    bindKey,
		IsActive:   true,
		MacAddress: deviceID, // 使用 deviceID 作为临时 MAC 地址，避免唯一约束冲突
		ClientID:   deviceID, // 使用 deviceID 作为临时 ClientID，避免唯一约束冲突
		CreateAt:   time.Now(),
		UpdateAt:   time.Now(),
	}

	return d.db.Create(&newBind).Error
}

// GetDevice 根据设备ID获取绑定信息
func (d *DeviceDB) GetDevice(deviceID string) (*models.Device, error) {
	var bind models.Device
	err := d.db.Where("device_id = ? AND is_active = ?", deviceID, true).First(&bind).Error
	if err != nil {
		return nil, err
	}
	return &bind, nil
}

// IsDeviceBound 检查设备是否已绑定
func (d *DeviceDB) IsDeviceBound(deviceID string) (bool, error) {
	var count int64
	err := d.db.Model(&models.Device{}).Where("device_id = ? AND is_active = ?", deviceID, true).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UnbindDevice 解绑设备
func (d *DeviceDB) UnbindDevice(deviceID string, bindKey string) error {
	return d.db.Model(&models.Device{}).Where("device_id = ? AND bind_key = ?", deviceID, bindKey).Update("is_active", false).Error
}

// GetUserDevices 获取用户绑定的所有设备
func (d *DeviceDB) GetUserDevices(userID uint) ([]models.Device, error) {
	var devices []models.Device
	err := d.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&devices).Error
	return devices, err
}

// UpdateDeviceStatus 更新设备状态与附加信息（仅更新已绑定、激活的设备）
func (d *DeviceDB) UpdateDeviceStatus(deviceID string, msgMap map[string]interface{}, userIDStr string) error {
	// 检查设备是否存在且已激活
	var existing models.Device
	if err := d.db.Where("device_id = ? AND is_active = ?", deviceID, true).First(&existing).Error; err != nil {
		return err
	}

	// 解析online（默认true）
	online := true
	if on, ok := msgMap["online"].(bool); ok {
		online = on
	}

	// 辅助解析函数
	getString := func(key string) (string, bool) {
		if v, ok := msgMap[key].(string); ok && strings.TrimSpace(v) != "" {
			return v, true
		}
		return "", false
	}
	getInt := func(key string) (int, bool) {
		if v, ok := msgMap[key].(float64); ok {
			return int(v), true
		}
		return 0, false
	}

	// 构造更新字段（列名按gorm snake_case）
	updates := map[string]interface{}{
		"online":              online,
		"last_active_time":    time.Now().Unix(),
		"last_active_time_v2": time.Now(),
		"update_at":           time.Now(),
	}

	if v, ok := getString("name"); ok {
		updates["name"] = v
	}
	if v, ok := getString("version"); ok {
		updates["version"] = v
	}
	if v, ok := getString("macAddress"); ok {
		updates["mac_address"] = v
	} else if v, ok := getString("mac"); ok {
		updates["mac_address"] = v
	}
	if v, ok := getString("clientId"); ok {
		updates["client_id"] = v
	} else if v, ok := getString("client_id"); ok {
		updates["client_id"] = v
	}
	if v, ok := getString("ssid"); ok {
		updates["ssid"] = v
	}
	if v, ok := getInt("channel"); ok {
		updates["channel"] = v
	}
	if v, ok := getString("language"); ok {
		updates["language"] = v
	}
	if v, ok := getString("application"); ok {
		updates["application"] = v
	}
	if v, ok := getString("boardType"); ok {
		updates["board_type"] = v
	} else if v, ok := getString("board_type"); ok {
		updates["board_type"] = v
	}
	if v, ok := getString("chipModelName"); ok {
		updates["chip_model_name"] = v
	} else if v, ok := getString("chip_model"); ok {
		updates["chip_model_name"] = v
	}
	if v, ok := getString("deviceCode"); ok {
		updates["device_code"] = v
	} else if v, ok := getString("device_code"); ok {
		updates["device_code"] = v
	}
	if v, ok := getString("mode"); ok {
		updates["mode"] = v
	}

	// 处理额外信息 extra（支持对象或字符串）
	if extraMap, ok := msgMap["extra"].(map[string]interface{}); ok {
		if b, err := json.Marshal(extraMap); err == nil {
			updates["extra"] = string(b)
		}
	} else if extraStr, ok := msgMap["extra"].(string); ok {
		updates["extra"] = extraStr
	}

	if err := d.db.Model(&models.Device{}).Where("device_id = ?", deviceID).Updates(updates).Error; err != nil {
		return err
	}

	// 可选：若提供user_id或连接传入的userID，则更新
	if uidStr, ok := getString("user_id"); ok {
		if uid64, err := strconv.ParseUint(uidStr, 10, 64); err == nil {
			_ = d.db.Model(&models.Device{}).Where("device_id = ?", deviceID).Update("user_id", uint(uid64)).Error
		}
	} else if strings.TrimSpace(userIDStr) != "" {
		if uid64, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
			_ = d.db.Model(&models.Device{}).Where("device_id = ?", deviceID).Update("user_id", uint(uid64)).Error
		}
	}

	return nil
}
