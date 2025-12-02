package mqtt

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MQTTConnection 实现 core.Connection 接口
// 代表与某个客户端ID对应的逻辑连接，通过特定 outTopic 发布消息，
// 通过 Transport 的订阅回调将消息注入到 incoming 队列。
// 可选支持UDP会话用于音频数据传输
type MQTTConnection struct {
	client     mqtt.Client
	id         string
	connType   string
	outTopic   string
	qos        byte
	closed     int32
	lastActive int64

	// UDP会话（可选，用于音频数据传输）
	udpSession *UDPSession
	udpServer  string // UDP服务器地址
	udpPort    string // UDP服务器端口

	incoming chan struct {
		messageType int
		data        []byte
	}
	mu sync.Mutex
}

func NewMQTTConnection(client mqtt.Client, id string, outTopic string, qos int) *MQTTConnection {
	c := &MQTTConnection{
		client:   client,
		id:       id,
		connType: "mqtt",
		outTopic: outTopic,
		qos:      byte(qos),
		incoming: make(chan struct {
			messageType int
			data        []byte
		}, 1024),
		lastActive: time.Now().UnixNano(),
	}
	return c
}

// SetUDPSession 设置UDP会话（用于音频数据传输）
func (c *MQTTConnection) SetUDPSession(session *UDPSession, server, port string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.udpSession = session
	c.udpServer = server
	c.udpPort = port
}

// GetUDPSession 获取UDP会话
// 返回 interface{} 以避免与 core 包产生循环依赖
func (c *MQTTConnection) GetUDPSession() interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.udpSession
}

// GetUDPInfo 实现UDPInfoProvider接口，获取UDP配置信息
func (c *MQTTConnection) GetUDPInfo() (enabled bool, server, port, key, nonce string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.udpSession == nil {
		return false, "", "", "", ""
	}

	keyHex, nonceHex := c.udpSession.GetAESKeyAndNonce()
	return true, c.udpServer, c.udpPort, keyHex, nonceHex
}

// WriteMessage 发布数据到 outTopic
// 如果是音频数据(messageType=2)且UDP会话活跃，优先使用UDP发送
func (c *MQTTConnection) WriteMessage(messageType int, data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return fmt.Errorf("连接已关闭")
	}

	// 如果是音频数据且UDP会话活跃，优先使用UDP发送
	if messageType == 2 {
		c.mu.Lock()
		udpSession := c.udpSession
		c.mu.Unlock()

		if udpSession != nil && udpSession.IsActive() {
			// 尝试通过UDP发送音频
			ok, err := udpSession.SendAudioData(data)
			if ok {
				atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
				fmt.Printf("✓ UDP发送音频成功: connID=%s, size=%d\n", c.id, len(data))
				return nil
			}
			// UDP发送失败，记录日志并回退到MQTT
			fmt.Printf("⚠ UDP发送音频失败，回退到MQTT: connID=%s, error=%v\n", c.id, err)
		}
	}

	// 控制消息(messageType=1)或UDP不可用，使用MQTT发送
	token := c.client.Publish(c.outTopic, c.qos, false, data)
	if token == nil {
		return fmt.Errorf("写入失败")
	}
	if ok := token.WaitTimeout(5 * time.Second); !ok {
		return fmt.Errorf("写入失败或超时")
	}
	if err := token.Error(); err != nil {
		return err
	}
	atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
	return nil
}

// ReadMessage 从内部队列读取一条消息；stopChan 关闭时返回
// 如果启用了UDP会话，同时监听UDP音频数据通道
func (c *MQTTConnection) ReadMessage(stopChan <-chan struct{}) (int, []byte, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return 0, nil, fmt.Errorf("连接已关闭")
	}

	// 检查是否有活跃的UDP会话
	c.mu.Lock()
	udpSession := c.udpSession
	c.mu.Unlock()

	if udpSession != nil && udpSession.IsActive() {
		// 同时监听MQTT信令和UDP音频数据
		select {
		case m := <-c.incoming:
			atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
			return m.messageType, m.data, nil
		case audioData, ok := <-udpSession.RecvChannel:
			if !ok {
				// UDP通道已关闭，回退到只监听MQTT
				break
			}
			atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
			return 2, audioData, nil // 返回二进制消息类型（音频数据）
		case <-stopChan:
			return 0, nil, fmt.Errorf("连接已关闭")
		}
	}

	// 没有UDP会话或UDP会话不活跃，只监听MQTT
	select {
	case m := <-c.incoming:
		atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
		return m.messageType, m.data, nil
	case <-stopChan:
		return 0, nil, fmt.Errorf("连接已关闭")
	}
}

// Close 关闭逻辑连接
func (c *MQTTConnection) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		c.mu.Lock()
		defer c.mu.Unlock()
		close(c.incoming)
	}
	return nil
}

func (c *MQTTConnection) GetID() string   { return c.id }
func (c *MQTTConnection) GetType() string { return c.connType }
func (c *MQTTConnection) IsClosed() bool  { return atomic.LoadInt32(&c.closed) == 1 }
func (c *MQTTConnection) GetLastActiveTime() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.lastActive))
}
func (c *MQTTConnection) IsStale(timeout time.Duration) bool {
	return time.Since(c.GetLastActiveTime()) > timeout
}

// PushIncoming 由 Transport 在收到订阅消息时调用
func (c *MQTTConnection) PushIncoming(messageType int, data []byte) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return
	}

	if messageType == 2 {
		handled, processed := c.handleIncomingUDPPacket(data)
		if handled {
			return
		}
		if processed != nil {
			data = processed
		}
	}
	select {
	case c.incoming <- struct {
		messageType int
		data        []byte
	}{messageType: messageType, data: data}:
		atomic.StoreInt64(&c.lastActive, time.Now().UnixNano())
	default:
		// 队列满丢弃，避免阻塞
	}
}

// handleIncomingUDPPacket 处理通过MQTT传输的UDP格式音频包
// 返回值 handled 表示数据已通过UDP会话通道处理完毕；processed 为需要继续走MQTT流程的payload
func (c *MQTTConnection) handleIncomingUDPPacket(payload []byte) (handled bool, processed []byte) {
	if len(payload) < 16 || payload[0] != 0x01 {
		return false, payload
	}

	c.mu.Lock()
	udpSession := c.udpSession
	c.mu.Unlock()

	if udpSession == nil || !udpSession.IsActive() {
		return false, payload[16:]
	}

	decrypted, err := udpSession.Decrypt(payload)
	if err != nil {
		if len(payload) > 16 {
			return false, payload[16:]
		}
		return false, payload
	}

	if ok, err := udpSession.RecvData(decrypted); ok {
		return true, nil
	} else if err != nil {
		fmt.Printf("✗ MQTT UDP投递RecvChannel失败: conn=%s, err=%v，回退到MQTT\n", c.id, err)
	}

	return false, decrypted
}
