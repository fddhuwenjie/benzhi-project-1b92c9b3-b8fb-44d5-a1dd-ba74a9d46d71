package storage

// cloneStrings 返回独立切片，避免调用方修改存储快照中的集合字段。
func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

// cloneIncidentSnapshot 复制异常快照中的复合字段，隔离查询调用方与存储状态。
func cloneIncidentSnapshot(in Incident) Incident {
	out := in
	if in.History != nil {
		out.History = append([]HistorySummary(nil), in.History...)
	}
	if in.LatestChange != nil {
		change := *in.LatestChange
		out.LatestChange = &change
	}
	return out
}
