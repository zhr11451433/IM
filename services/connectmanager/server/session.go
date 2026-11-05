package server

//记录一个 WebSocket 连接对应的用户身份和设备信息

type Session struct {
	AppKey   string
	UserID   string
	DeviceID string
	Platform string
}

func (s *ImWebsocketServer) Count(appKey, userID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, session := range s.sessions {
		if session.AppKey == appKey && session.UserID == userID {
			count++
		}
	}
	return count
}

//目的就是关闭 WebSocket Server
//当前管理的所有 WebSocket 连接，而不是只关闭某一个连接

func (s *ImWebsocketServer) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		for conn := range s.conns {
			_ = conn.Close()
		}
	}
	s.mu.Unlock()

	s.wg.Wait()
}
