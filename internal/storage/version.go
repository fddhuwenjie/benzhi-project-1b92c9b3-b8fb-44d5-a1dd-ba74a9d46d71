package storage

// snapshotVersion 是本地快照格式的单一版本来源。
const snapshotVersion = 1

func supportedSnapshotVersion(v int) bool { return v == snapshotVersion }
