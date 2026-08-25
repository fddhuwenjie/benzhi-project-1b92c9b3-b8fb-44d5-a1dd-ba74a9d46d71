package storage

import "path/filepath"

func snapshotPath(dir string) string     { return filepath.Join(dir, "snapshot.json") }
func snapshotTempPath(dir string) string { return filepath.Join(dir, "snapshot.json.tmp") }
func eventsPath(dir string) string       { return filepath.Join(dir, "events.log") }
