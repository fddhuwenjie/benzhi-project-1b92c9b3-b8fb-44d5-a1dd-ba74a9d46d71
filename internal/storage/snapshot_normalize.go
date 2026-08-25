package storage

// normalizeSnapshot 补齐旧快照中新增的原始读数和发现说明字段。
func normalizeSnapshot(data *snapshot) {
	for n := range data.Incidents {
		if data.Incidents[n].OriginalDiscovery == "" {
			data.Incidents[n].OriginalDiscovery = data.Incidents[n].Discovery
		}
		if data.Incidents[n].OriginalReading == 0 && data.Incidents[n].Reading != 0 {
			data.Incidents[n].OriginalReading = data.Incidents[n].Reading
		}
	}
}
