package storage

// Directory 返回当前存储目录，便于诊断和运维工具展示。
func (s *Store) Directory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dir
}
