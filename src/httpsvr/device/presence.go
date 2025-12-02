package device

import (
    "sync"
    "time"
)

// DeviceSessionPresence 记录单个会话的在线状态与最后活跃时间
type DeviceSessionPresence struct {
    SessionID   string
    Online      bool
    LastActive  time.Time
}

// HeartbeatMetrics 心跳指标（可按需扩展）
type HeartbeatMetrics struct {
    Timestamp int64
    Battery   float64
    Temp      float64
    Net       string
    RSSI      int
}

// DevicePresence 记录设备维度的在线状态与心跳信息
type DevicePresence struct {
    DeviceID       string
    Online         bool
    LastHeartbeat  time.Time
    LastConnEvt    time.Time
    Battery        float64
    Temp           float64
    Net            string
    RSSI           int
    Sessions       map[string]*DeviceSessionPresence
}

// PresenceManager 管理设备在线/离线与会话状态
type PresenceManager struct {
    mu      sync.RWMutex
    devices map[string]*DevicePresence
}

var defaultPresenceManager = &PresenceManager{devices: make(map[string]*DevicePresence)}

// GetPresenceManager 获取默认 PresenceManager（单例）
func GetPresenceManager() *PresenceManager { return defaultPresenceManager }

// ensureDevice 初始化设备结构
func (pm *PresenceManager) ensureDevice(deviceID string) *DevicePresence {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    dp, ok := pm.devices[deviceID]
    if !ok {
        dp = &DevicePresence{DeviceID: deviceID, Sessions: make(map[string]*DeviceSessionPresence)}
        pm.devices[deviceID] = dp
    }
    return dp
}

// SetSessionOnline 标记会话在线
func (pm *PresenceManager) SetSessionOnline(deviceID, sessionID string) {
    dp := pm.ensureDevice(deviceID)
    pm.mu.Lock()
    defer pm.mu.Unlock()
    sess, ok := dp.Sessions[sessionID]
    if !ok {
        sess = &DeviceSessionPresence{SessionID: sessionID}
        dp.Sessions[sessionID] = sess
    }
    sess.Online = true
    sess.LastActive = time.Now()
    // 设备层在线：有任意会话在线即认为设备在线
    dp.Online = true
    dp.LastConnEvt = time.Now()
}

// SetSessionOffline 标记会话离线
func (pm *PresenceManager) SetSessionOffline(deviceID, sessionID string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    if dp, ok := pm.devices[deviceID]; ok {
        if sess, ok2 := dp.Sessions[sessionID]; ok2 {
            sess.Online = false
        }
        // 若所有会话均离线，则设备离线
        dp.Online = false
        for _, s := range dp.Sessions {
            if s.Online {
                dp.Online = true
                break
            }
        }
        dp.LastConnEvt = time.Now()
    }
}

// TouchSession 更新会话活跃时间
func (pm *PresenceManager) TouchSession(deviceID, sessionID string) {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    if dp, ok := pm.devices[deviceID]; ok {
        if sess, ok2 := dp.Sessions[sessionID]; ok2 {
            sess.LastActive = time.Now()
        }
    }
}

// UpdateHeartbeat 更新设备心跳与指标
func (pm *PresenceManager) UpdateHeartbeat(deviceID string, hb HeartbeatMetrics) {
    dp := pm.ensureDevice(deviceID)
    pm.mu.Lock()
    defer pm.mu.Unlock()
    if hb.Timestamp > 0 {
        dp.LastHeartbeat = time.Unix(hb.Timestamp, 0)
    } else {
        dp.LastHeartbeat = time.Now()
    }
    if hb.Battery != 0 { dp.Battery = hb.Battery }
    if hb.Temp != 0 { dp.Temp = hb.Temp }
    if hb.Net != "" { dp.Net = hb.Net }
    if hb.RSSI != 0 { dp.RSSI = hb.RSSI }
    // 收到心跳也可认为设备在线
    dp.Online = true
}

// SetDeviceConnectionState 设置设备连接状态（用于 LWT 与显式上线/下线）
func (pm *PresenceManager) SetDeviceConnectionState(deviceID string, online bool) {
    dp := pm.ensureDevice(deviceID)
    pm.mu.Lock()
    defer pm.mu.Unlock()
    dp.Online = online
    dp.LastConnEvt = time.Now()
}

// GetDevicePresence 获取设备在线信息（不可修改原始对象）
func (pm *PresenceManager) GetDevicePresence(deviceID string) *DevicePresence {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    if dp, ok := pm.devices[deviceID]; ok {
        // 返回浅拷贝避免外部修改内部状态
        copy := *dp
        copy.Sessions = make(map[string]*DeviceSessionPresence)
        for k, v := range dp.Sessions {
            cv := *v
            copy.Sessions[k] = &cv
        }
        return &copy
    }
    return nil
}