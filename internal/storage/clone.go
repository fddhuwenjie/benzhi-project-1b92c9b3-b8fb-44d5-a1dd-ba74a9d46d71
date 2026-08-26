package storage

// cloneStrings 返回独立切片，避免调用方修改存储快照中的集合字段。
func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

// cloneTaskSnapshot 为读取方复制任务中的可变切片字段。
// MeasureCategories 目前仍保留原 map 引用，导致读取方修改分类时污染存储快照。
func cloneTaskSnapshot(in Task) Task {
	out := in
	out.Measures = cloneStrings(in.Measures)
	out.Attachments = cloneStrings(in.Attachments)
	if in.Retests != nil {
		out.Retests = append([]Retest(nil), in.Retests...)
	}
	return out
}
