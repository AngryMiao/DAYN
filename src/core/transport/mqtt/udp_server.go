package mqtt

import (
	"angrymiao-ai-server/src/configs"
	"angrymiao-ai-server/src/core/utils"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// UDPServer UDP服务器，负责处理UDP音频数据的接收和发送
type UDPServer struct {
	conn          *net.UDPConn  // UDP连接
	listenPort    int           // UDP监听端口
	externalHost  string        // 外部访问地址（返回给客户端）
	externalPort  int           // 外部访问端口（返回给客户端）
	nonce2Session sync.Map      // connID -> *UDPSession
	addr2Session  sync.Map      // remoteAddr.String() -> *UDPSession
	logger        *utils.Logger // 日志记录器
	stopChan      chan struct{} // 停止信号
	stopOnce      sync.Once
	wg            sync.WaitGroup // 等待goroutine结束
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isHealthCheckPacket 判断是否是健康检查包
// 健康检查包通常是明文ASCII字符，如 "Healthcheck udp check"
func isHealthCheckPacket(data []byte) bool {
	if len(data) == 0 || len(data) > 100 {
		// 健康检查包通常很短，超过100字节的不太可能是健康检查
		return false
	}

	// 检查前几个字节是否都是可打印的ASCII字符
	checkLen := min(len(data), 30)
	printableCount := 0
	for i := 0; i < checkLen; i++ {
		// 可打印ASCII字符范围：0x20-0x7E，加上常见的空白字符
		if (data[i] >= 0x20 && data[i] <= 0x7E) || data[i] == 0x09 || data[i] == 0x0A || data[i] == 0x0D {
			printableCount++
		}
	}

	// 如果超过80%的字符都是可打印字符，认为是健康检查包
	return float64(printableCount)/float64(checkLen) > 0.8
}

func (s *UDPServer) isStopping() bool {
	select {
	case <-s.stopChan:
		return true
	default:
		return false
	}
}

// NewUDPServer 创建新的UDP服务器
func NewUDPServer(cfg *configs.Config, logger *utils.Logger) *UDPServer {
	udpCfg := cfg.Transport.Mqtt.UDP
	return &UDPServer{
		listenPort:   udpCfg.ListenPort,
		externalHost: udpCfg.ExternalHost,
		externalPort: udpCfg.ExternalPort,
		logger:       logger,
		stopChan:     make(chan struct{}),
	}
}

// Start 启动UDP服务器
func (s *UDPServer) Start() error {
	addr := &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: s.listenPort,
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("UDP服务器启动失败: %v", err)
	}

	s.conn = conn
	s.logger.Info("UDP服务器已启动: 监听端口=%d, 外部地址=%s:%d",
		s.listenPort, s.externalHost, s.externalPort)

	// 启动数据包处理goroutine
	s.wg.Add(1)
	go s.handlePackets()

	return nil
}

// Stop 停止UDP服务器
func (s *UDPServer) Stop() error {
	var stopErr error
	s.stopOnce.Do(func() {
		s.logger.Info("正在停止UDP服务器...")

		// 发送停止信号
		close(s.stopChan)

		// 关闭UDP连接
		if s.conn != nil {
			if err := s.conn.Close(); err != nil {
				stopErr = err
			}
		}

		// 等待所有goroutine结束
		s.wg.Wait()

		// 清理所有会话
		s.nonce2Session.Range(func(key, value interface{}) bool {
			if session, ok := value.(*UDPSession); ok {
				session.Destroy()
			}
			s.nonce2Session.Delete(key)
			return true
		})

		s.addr2Session.Range(func(key, value interface{}) bool {
			s.addr2Session.Delete(key)
			return true
		})

		s.logger.Info("UDP服务器已停止")
	})
	return stopErr
}

// CreateSession 创建新的UDP会话
func (s *UDPServer) CreateSession(deviceID, sessionID string) (*UDPSession, error) {
	// 生成16字节AES密钥
	aesKey, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("生成AES密钥失败: %v", err)
	}

	// 生成4字节连接ID
	connIDBytes, err := GenerateConnID()
	if err != nil {
		return nil, fmt.Errorf("生成连接ID失败: %v", err)
	}

	// 生成8字节nonce模板
	nonceTemplate := GenerateNonceTemplate(connIDBytes)

	// 创建会话
	connIDHex := hex.EncodeToString(connIDBytes[:])
	session, err := NewUDPSession(deviceID, sessionID, aesKey, nonceTemplate, connIDHex)
	if err != nil {
		return nil, fmt.Errorf("创建UDP会话失败: %v", err)
	}

	// 存储会话映射（使用connID的前4字节作为key）
	s.nonce2Session.Store(connIDHex, session)

	// 启动发送goroutine
	s.wg.Add(1)
	go s.handleSend(session)

	// keyHex, nonceHex := session.GetAESKeyAndNonce()
	// s.logger.Info("✓ 创建UDP会话: deviceID=%s, sessionID=%s, connID=%s, server=%s:%d, key=%s, nonce=%s",
	// 	deviceID, sessionID, connIDHex, s.externalHost, s.externalPort, keyHex, nonceHex)

	return session, nil
}

// ProbeClientAddress 主动探测客户端UDP地址（用于NAT穿透）
// 向客户端提供的公网IP:Port发送探测包，等待客户端回复以确认真实地址
func (s *UDPServer) ProbeClientAddress(session *UDPSession, clientIP string, clientPort int) error {
	if clientIP == "" || clientPort == 0 {
		return fmt.Errorf("客户端地址信息不完整")
	}

	targetAddr := &net.UDPAddr{
		IP:   net.ParseIP(clientIP),
		Port: clientPort,
	}

	s.logger.Info("开始UDP地址探测: connID=%s, 目标地址=%s", session.ConnID, targetAddr.String())

	// 构造探测包：使用特殊标记 0x02 表示探测包
	probeData := make([]byte, 16)
	probeData[0] = 0x02                              // 探测包标记
	copy(probeData[1:5], []byte(session.ConnID[:8])) // 复制connID的前4字节

	// 发送探测包（带重试）
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		_, err := s.conn.WriteToUDP(probeData, targetAddr)
		if err == nil {
			s.logger.Info("UDP探测包已发送: 目标=%s, 重试=%d/%d", targetAddr.String(), retry+1, maxRetries)
			break
		}

		if retry < maxRetries-1 {
			s.logger.Warn("UDP探测包发送失败，重试: 目标=%s, error=%v", targetAddr.String(), err)
			time.Sleep(100 * time.Millisecond)
		} else {
			return fmt.Errorf("UDP探测包发送失败: %v", err)
		}
	}

	// 注意：不在这里等待回复，回复会通过正常的UDP接收流程处理
	// 客户端收到探测包后会回复一个正常的UDP包，服务端会记录其源地址
	return nil
}

// CloseSession 关闭会话
func (s *UDPServer) CloseSession(connID string) {
	if value, ok := s.nonce2Session.Load(connID); ok {
		if session, ok := value.(*UDPSession); ok {
			s.logger.Info("关闭UDP会话: connID=%s", connID)
			session.Destroy()

			// 从映射中删除
			s.nonce2Session.Delete(connID)

			// 从地址映射中删除
			if session.RemoteAddr != nil {
				s.addr2Session.Delete(session.RemoteAddr.String())
			}
		}
	}
}

// getSessionByNonce 根据connID查找会话
func (s *UDPServer) getSessionByNonce(connID string) (*UDPSession, bool) {
	if value, ok := s.nonce2Session.Load(connID); ok {
		if session, ok := value.(*UDPSession); ok {
			return session, true
		}
	}
	return nil, false
}

// getUdpSession 根据远程地址查找会话
func (s *UDPServer) getUdpSession(addr *net.UDPAddr) (*UDPSession, bool) {
	if value, ok := s.addr2Session.Load(addr.String()); ok {
		if session, ok := value.(*UDPSession); ok {
			return session, true
		}
	}
	return nil, false
}

// addUdpSession 添加地址到会话的映射
func (s *UDPServer) addUdpSession(addr *net.UDPAddr, session *UDPSession) {
	s.addr2Session.Store(addr.String(), session)
	s.logger.Debug("记录UDP地址映射: addr=%s, connID=%s", addr.String(), session.ConnID)
}

// handlePackets 处理接收到的UDP数据包
func (s *UDPServer) handlePackets() {
	defer s.wg.Done()

	buffer := make([]byte, 65535) // UDP最大包大小

	for {
		select {
		case <-s.stopChan:
			return
		default:
			// 设置读取超时，避免阻塞
			s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, remoteAddr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// 超时，继续循环
					continue
				}
				if errors.Is(err, net.ErrClosed) || s.isStopping() {
					s.logger.Debug("UDP连接已关闭，退出监听")
					return
				}
				s.logger.Error("读取UDP数据包失败: %v", err)
				continue
			}

			// 处理数据包
			s.processPacket(remoteAddr, buffer[:n])
		}
	}
}

// processPacket 处理单个UDP数据包
func (s *UDPServer) processPacket(addr *net.UDPAddr, data []byte) {
	if len(data) < 16 {
		s.logger.Warn("UDP数据包长度不足: addr=%s, len=%d", addr.String(), len(data))
		return
	}

	// 提取nonce信息
	nonce := data[0:16]

	// 验证包类型：第一个字节必须是0x01
	if nonce[0] != 0x01 {
		// 这可能是健康检查或其他非加密包（阿里云集群会给UDP发送心跳）
		// 检查是否是可打印的ASCII字符（健康检查通常是明文）
		if isHealthCheckPacket(data) {
			// 静默忽略健康检查包，不打印日志以减少噪音
			// 如果需要调试，可以取消下面的注释
			// s.logger.Debug("忽略健康检查包: addr=%s", addr.String())
			return
		}
		// 其他非标准包，记录警告
		s.logger.Warn("收到非标准UDP包: addr=%s, 首字节=0x%02x", addr.String(), nonce[0])
		return
	}

	connIDBytes, seq, dataLen, err := ExtractNonceInfo(nonce)
	if err != nil {
		s.logger.Warn("解析nonce失败: addr=%s, error=%v", addr.String(), err)
		return
	}

	connID := hex.EncodeToString(connIDBytes)

	// 查找会话
	session, ok := s.getSessionByNonce(connID)
	if !ok {
		s.logger.Warn("未找到UDP会话: addr=%s, connID=%s", addr.String(), connID)
		return
	}

	// 记录或更新设备地址映射
	if session.RemoteAddr == nil {
		// 首次记录地址
		session.RemoteAddr = addr
		s.addUdpSession(addr, session)
		s.logger.Info("记录设备UDP地址: deviceID=%s, addr=%s", session.DeviceID, addr.String())
	} else if session.RemoteAddr.String() != addr.String() {
		// 设备地址变化（如4G网络切换），更新映射
		oldAddr := session.RemoteAddr.String()
		s.addr2Session.Delete(oldAddr) // 删除旧地址映射
		session.RemoteAddr = addr
		s.addUdpSession(addr, session)
		s.logger.Info("更新设备UDP地址: deviceID=%s, 旧地址=%s, 新地址=%s",
			session.DeviceID, oldAddr, addr.String())
	}

	// 解密数据
	decrypted, err := session.Decrypt(data)
	if err != nil {
		s.logger.Warn("解密UDP数据包失败: addr=%s, connID=%s, seq=%d, error=%v",
			addr.String(), connID, seq, err)
		return
	}

	// 验证数据长度
	if len(decrypted) != int(dataLen) {
		s.logger.Warn("解密后数据长度不匹配: 期望=%d, 实际=%d", dataLen, len(decrypted))
		return
	}

	// 检查解密后的数据是否包含 nonce 头部（前16字节）
	actualAudioData := decrypted
	if len(decrypted) >= 16 && decrypted[0] == 0x01 {
		// 去除前16字节的 nonce 头部
		actualAudioData = decrypted[16:]
	}

	// 更新会话活跃时间
	session.mu.Lock()
	session.LastActive = time.Now()
	session.mu.Unlock()

	// 投递到接收通道
	ok, err = session.RecvData(actualAudioData)
	if !ok {
		s.logger.Warn("投递音频数据失败: connID=%s, error=%v", connID, err)
	}
}

// handleSend 处理发送队列（每个会话一个goroutine）
func (s *UDPServer) handleSend(session *UDPSession) {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan:
			return
		case data, ok := <-session.SendChannel:
			if !ok {
				// 通道已关闭
				return
			}

			// 等待RemoteAddr被设置
			if session.RemoteAddr == nil {
				s.logger.Warn("UDP会话尚未建立地址映射，丢弃数据: connID=%s", session.ConnID)
				continue
			}

			// 加密数据
			encrypted, err := session.Encrypt(data)
			if err != nil {
				s.logger.Error("加密UDP数据失败: connID=%s, error=%v", session.ConnID, err)
				continue
			}

			// 发送UDP数据包（带重试）
			maxRetries := 3
			for retry := 0; retry < maxRetries; retry++ {
				_, err = s.conn.WriteToUDP(encrypted, session.RemoteAddr)
				if err == nil {
					break // 发送成功
				}

				if errors.Is(err, net.ErrClosed) || s.isStopping() {
					s.logger.Debug("UDP发送已停止: connID=%s", session.ConnID)
					return
				}

				if retry < maxRetries-1 {
					s.logger.Warn("UDP发送失败，重试 %d/%d: addr=%s, error=%v",
						retry+1, maxRetries-1, session.RemoteAddr.String(), err)
					time.Sleep(10 * time.Millisecond) // 短暂延迟后重试
				}
			}

			if err != nil {
				s.logger.Error("UDP发送失败（已重试%d次）: addr=%s, error=%v",
					maxRetries, session.RemoteAddr.String(), err)
				continue
			}

			s.logger.Info("✓ 发送UDP音频数据: addr=%s, connID=%s, size=%d",
				session.RemoteAddr.String(), session.ConnID, len(data))
		}
	}
}
