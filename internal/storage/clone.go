package storage

// cloneStrings 返回独立切片，避免调用方修改存储快照中的集合字段。
func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}
