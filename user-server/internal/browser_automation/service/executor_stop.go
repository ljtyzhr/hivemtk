package service

// executor_stop.go —— 手动中断 / 停止通道注册表（与 D7 确认闸门同构、语义相反）。
// SignalStop 手动中断（POST /sessions/:id/stop）
func (e *Executor) SignalStop(sessionID uint) bool {
	e.stopMu.Lock()
	ch, ok := e.stopRegistry[sessionID]
	e.stopMu.Unlock()
	if !ok {
		return false
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
	return true
}

func (e *Executor) registerStop(sessionID uint) chan struct{} {
	ch := make(chan struct{})
	e.stopMu.Lock()
	e.stopRegistry[sessionID] = ch
	e.stopMu.Unlock()
	return ch
}

func (e *Executor) unregisterStop(sessionID uint) {
	e.stopMu.Lock()
	delete(e.stopRegistry, sessionID)
	e.stopMu.Unlock()
}

func (e *Executor) stopFired(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// stopChFor 读取 session 当前注册的 stop 通道（分发路径不穿 stopCh 参数，finalize 轮询需可中断）。
// 未注册返回 nil——channel(nil) 的接收永远阻塞，调用方须先判空。
func (e *Executor) stopChFor(sessionID uint) chan struct{} {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	return e.stopRegistry[sessionID]
}
